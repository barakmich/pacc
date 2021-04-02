// Wrapping libav for AAC encoding.
// Largely cribbed from the example code of FFmpeg, since
// it doesn't need to do much that's fancy.
package aac

import (
	/*
		#cgo LDFLAGS: -lavformat -lavutil -lavcodec
		#include <stdio.h>
		#include <libavcodec/avcodec.h>
		#include <libavutil/avutil.h>

		typedef struct {
			AVCodec *codec;
			AVCodecContext *ctx;
			AVPacket *pkt;
			int samplerate; int bitrate;
			int channels;
		} aacenc_t ;

		static int aacenc_new(aacenc_t *m) {
			m->codec = avcodec_find_encoder(AV_CODEC_ID_AAC);
			if (!m->codec) {
				fprintf(stderr, "Codec not found\n");
				return -2;
			}
			m->ctx = avcodec_alloc_context3(m->codec);
			m->ctx->bit_rate = m->bitrate;
			m->ctx->sample_fmt = AV_SAMPLE_FMT_S16;
			m->ctx->sample_rate = m->samplerate;
			m->ctx->channels = m->channels;
			if (avcodec_open2(m->ctx, m->codec, 0) < 0) {
				fprintf(stderr, "Couldn't open codec\n");
				return -2;
			}
			m->pkt = av_packet_alloc();
			if (!m->pkt) {
				fprintf(stderr, "Couldn't alloc packet\n");
				return -2;
			}
			return 0;
		}

		static int aacenc_encode(aacenc_t *m, uint8_t* data, size_t len, int samples) {
			AVFrame *frame;
			int ret;
			int n;
			frame = av_frame_alloc();
			if (!frame) {
				fprintf(stderr, "Couldn't alloc frame\n");
				return -2;
			}
			frame->nb_samples = samples;
			frame->format = m->ctx->sample_fmt;
			frame->channel_layout = m->ctx->channels == 2 ? AV_CH_LAYOUT_STEREO : AV_CH_LAYOUT_MONO;
			ret = av_frame_get_buffer(frame, 0);
			if (ret < 0) {
				fprintf(stderr, "Couldn't alloc framebuffer\n");
				av_frame_free(&frame);
				return ret;
			}
			ret = av_frame_make_writable(frame);
			if (ret < 0) {
				fprintf(stderr, "Couldn't writeable framebuffer\n");
				av_frame_free(&frame);
				return ret;
			}
			memcpy(frame->data[0], data, len);
			ret = avcodec_send_frame(m->ctx, frame);
			if (ret < 0) {
				fprintf(stderr, "Couldn't send frame\n");
			}
			av_frame_free(&frame);
			return ret;
		}

		static int nextpkt(aacenc_t* m) {
			int ret = avcodec_receive_packet(m->ctx, m->pkt);
			if (ret == AVERROR(EAGAIN) || ret == AVERROR_EOF) return -1;
			else if (ret < 0) {
				fprintf(stderr, "Error encoding frame\n");
				return -2;
			}
			return ret;
		}

		static int finishpkt(aacenc_t* m) {
			av_packet_unref(m->pkt);
		}

		static int aacenc_close(aacenc_t *m) {
			av_packet_free(&m->pkt);
			avcodec_free_context(&m->ctx);
		}

	*/
	"C"
	"errors"
	"unsafe"
)
import "io"

type Encoder struct {
	m        C.aacenc_t
	Header   []byte
	Writer   io.Writer
	buf      []byte
	Channels int
}

type EncoderConfig struct {
	SampleRate    int
	OutputBitrate int
	Channels      int
	WriteHeader   bool
}

const SAMPLE_BITRATE = 16

func DefaultConfig() EncoderConfig {
	return EncoderConfig{
		SampleRate:    44100,
		OutputBitrate: 50000,
		Channels:      2,
		WriteHeader:   true,
	}
}

func NewEncoder(writer io.Writer, conf EncoderConfig) (*Encoder, error) {
	m := &Encoder{
		Writer:   writer,
		Channels: conf.Channels,
	}
	m.m.samplerate = C.int(conf.SampleRate)
	m.m.bitrate = C.int(conf.OutputBitrate)
	m.m.channels = C.int(conf.Channels)
	r := C.aacenc_new(&m.m)
	if int(r) < 0 {
		err := errors.New("open codec failed")
		return nil, err
	}
	m.Header = make([]byte, (int)(m.m.ctx.extradata_size))
	C.memcpy(
		unsafe.Pointer(&m.Header[0]),
		unsafe.Pointer(&m.m.ctx.extradata),
		(C.size_t)(len(m.Header)),
	)
	if conf.WriteHeader {
		if err := m.WriteHeader(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Encoder) WriteHeader() error {
	_, err := m.Writer.Write(m.Header)
	return err
}

func (m *Encoder) Write(samples []byte) (n int, err error) {
	data := (*C.uint8_t)(unsafe.Pointer(&samples[0]))
	ret := int(C.aacenc_encode(&m.m, data, (C.size_t)(len(samples)), (C.int)(len(samples)/m.Channels)))
	if ret < 0 {
		return 0, errors.New("error with aacenc_encode")
	}
	for ret >= 0 {
		ret = int(C.nextpkt(&m.m))
		if ret == -1 {
			// Done!
			return
		} else if ret == -2 {
			panic("error encoding audio frame")
		}
		size := (int)(m.m.pkt.size)
		if size > len(m.buf) {
			m.resizeBuf(size)
		}
		C.memcpy(
			unsafe.Pointer(&m.buf[0]),
			unsafe.Pointer(m.m.pkt.data),
			(C.size_t)(m.m.pkt.size),
		)
		out, err := m.Writer.Write(m.buf[:size])
		n += out
		if err != nil {
			return n, err
		}
		C.finishpkt(&m.m)
	}
	return n, nil
}

func (m *Encoder) resizeBuf(n int) {
	m.buf = make([]byte, n)
}
