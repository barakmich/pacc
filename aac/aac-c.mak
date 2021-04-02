#include <stdio.h>
#include <libavcodec/avcodec.h>
#include <libavutil/avutil.h>

int main() {
	AVCodec* codec;
	AVCodecContext* ctx = NULL;
	codec = avcodec_find_encoder(AV_CODEC_ID_FLAC);
	if (!codec) {
		fprintf(stderr, "Codec not found\n");
		return -2;
	}
	ctx = avcodec_alloc_context3(codec);
	printf("%d\n", ctx->frame_size);
	return 0;
}
