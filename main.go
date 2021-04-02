package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	cast "github.com/barakmich/gochromecast"
)

const mimeType = "audio/flac"

type AudioSink struct {
	sync.Mutex
	sink      *NullSink
	closechan chan bool
	subs      []chan []byte
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
	w.Header().Set("Content-Type", mimeType)
	args := []string{
		"-f", "pulse", "-i", a.sink.Name + ".monitor", "-copytb", "1", "-f", "flac", "-",
	}
	fmt.Printf("%v\n", args)
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = w
	cmd.Stderr = os.Stdout
	err := cmd.Run()
	if err != nil {
		log.Println("Encoding error:", err)
	}
	fmt.Println("Connection done")
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
	http.Handle("/stream", sink)
	hostport := fmt.Sprintf("%s:8884", addr)
	go http.ListenAndServe(":8884", nil)
	fmt.Println("Listening on", hostport)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGKILL)
	device, err := initChromecast(os.Args[1], iface, hostport)
	if err != nil {
		log.Print(err)
		return
	}
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
		fmt.Sprintf("http://%s/stream", httpHostPort),
		mimeType,
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
	close(searchchan)
}

func makeSink(closer chan bool) (*AudioSink, error) {
	sink := &NullSink{
		Name: "PACC",
		//UseSystemClockForTiming: true,
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
	return out, nil
}
