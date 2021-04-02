package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/akosmarton/papipes"
	cast "github.com/barakmich/gochromecast"
	"github.com/gen2brain/aac-go"
)

type AudioSink struct {
	sync.Mutex
	sink      *papipes.Sink
	closechan chan bool
	subs      []chan []byte
}

type ChanbufReader struct {
	c      chan []byte
	cancel <-chan struct{}
	buf    []byte
}

func (cb *ChanbufReader) Read(out []byte) (int, error) {
	for len(cb.buf) < len(out) {
		select {
		case _, ok := <-cb.cancel:
			if !ok {
				log.Println("Cancel called")
			}
			return 0, io.EOF
		case buf := <-cb.c:
			cb.buf = append(cb.buf, buf...)
		}
	}
	n := copy(out, cb.buf)
	cb.buf = cb.buf[n:]
	return n, nil
}

func (a *AudioSink) Close() error {
	a.Lock()
	defer a.Unlock()
	for _, c := range a.subs {
		close(c)
	}
	return a.sink.Close()
}

func (a *AudioSink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Got connection")
	fmt.Println(r)
	fmt.Println(r.Header)
	w.Header().Set("Content-Type", "audio/aac")
	enc, err := aac.NewEncoder(w, &aac.Options{
		SampleRate:  44100,
		NumChannels: 2,
	})
	if err != nil {
		log.Fatal(err)
	}
	sub := a.subscribe()
	tr := &ChanbufReader{
		c:      sub,
		cancel: r.Cancel,
	}
	err = enc.Encode(tr)
	if err != nil {
		log.Println("Encoding error:", err)
	}
	a.unsubscribe(sub)
	fmt.Println("Connection done")
}

func (a *AudioSink) streamAll() {
	for {
		select {
		case <-a.closechan:
			return
		default:
			//fallthrough
		}
		buf := make([]byte, 4096)
		n, err := a.sink.Read(buf)
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Println("streamall", err)
		}
		for _, c := range a.subs {
			c <- buf[:n]
		}
	}
}

func (a *AudioSink) subscribe() chan []byte {
	a.Lock()
	defer a.Unlock()
	out := make(chan []byte, 100)
	a.subs = append(a.subs, out)
	return out
}

func (a *AudioSink) unsubscribe(sub chan []byte) error {
	a.Lock()
	defer a.Unlock()
	for i, c := range a.subs {
		if c == sub {
			a.subs = append(a.subs[:i], a.subs[i+1:]...)
			close(sub)
			return nil
		}
	}
	return errors.New("Couldn't unsubscribe channel")
}

func GetLocalIP(iface net.Interface) string {
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func main() {
	closer := make(chan bool)
	iface, err := upInterface()
	if err != nil {
		log.Fatal(err)
	}
	addr := GetLocalIP(iface)
	fmt.Println("Using addr", addr)

	sink, err := makeSink(closer)
	if err != nil {
		log.Fatal(err)
	}
	defer sink.Close()
	defer sink.sink.Close()
	defer close(closer)
	http.Handle("/stream.aac", sink)
	hostport := fmt.Sprintf("%s:8080", addr)
	go http.ListenAndServe(hostport, nil)
	fmt.Println("Listening on", hostport)

	device, err := initChromecast(os.Args[1], iface, hostport)
	if err != nil {
		log.Print(err)
		return
	}
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGKILL)
	<-c
	device.QuitApplication(5 * time.Second)
}

func initChromecast(name string, iface net.Interface, httpHostPort string) (cast.Device, error) {
	devices := make(chan CastEntry, 50)
	go findDevices(iface, devices, 5*time.Second)
	found := false
	var dev cast.Device
	var err error
	for entry := range devices {
		fmt.Println(entry.DeviceName)
		if entry.DeviceName == name {
			found = true
			fmt.Println("** Found")
			dev, err = cast.NewDevice(entry.AddrV4, entry.Port)
			if err != nil {
				return dev, err
			}
			break
		}
	}
	if !found {
		return dev, errors.New("Couldn't find device")
	}
	dev.PlayMedia(
		fmt.Sprintf("http://%s/stream.aac", httpHostPort),
		"audio/aac",
		"NONE",
	)
	return dev, nil
}

func upInterface() (net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, errors.New("Couldn't get interfaces")
	}
	for _, iface := range ifaces {
		if int(iface.Flags&net.FlagUp) == 0 {
			continue
		}
		if int(iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		return iface, nil

	}
	return net.Interface{}, errors.New("Couldn't find active interface")
}

func findDevices(iface net.Interface, searchchan chan CastEntry, timeout time.Duration) {
	to, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c, err := DiscoverCastDNSEntries(to, &iface)
	if err != nil {
		log.Fatal(err)
	}
	for m := range c {
		searchchan <- m
	}
	fmt.Println("Done discovering")
}

func makeSink(closer chan bool) (*AudioSink, error) {
	sink := &papipes.Sink{
		Filename:                "/tmp/pacc.sock",
		Name:                    "PACC",
		UseSystemClockForTiming: true,
	}
	sink.SetProperty("device.description", "PACC Output")
	err := sink.Open()
	if err != nil {
		return nil, err
	}
	out := &AudioSink{
		sink:      sink,
		closechan: closer,
	}
	go out.streamAll()
	return out, nil
}
