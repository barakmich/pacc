package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/akosmarton/papipes"
	"github.com/barakmich/pacc/icon"
	"github.com/gen2brain/aac-go"
	"github.com/getlantern/systray"
)

type ZeroBufWriter struct {
	r io.Reader
}

var searchchan = make(chan CastEntry)

func sysTrayMain() {
	go func() {
		systray.Run(onready, onexit)
	}()
	for x := range searchchan {
		systray.AddMenuItemCheckbox(x.DeviceName, "", false)
		fmt.Println(x.DeviceName)
	}

}

func onready() {
	systray.SetTitle("PACC")
	systray.SetIcon(icon.Data)
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(ifaces)
	for _, iface := range ifaces {
		if int(iface.Flags&net.FlagUp) == 0 {
			continue
		}
		if int(iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		c, err := DiscoverCastDNSEntries(context.TODO(), &iface)
		if err != nil {
			log.Fatal(err)
		}
		go func(c <-chan CastEntry) {
			for m := range c {
				searchchan <- m
			}
		}(c)
	}
}

func onexit() {

}

func paMain() {
	sink := &papipes.Sink{
		Filename: "/tmp/pacca.sock",
		Name:     "PACC",
	}
	sink.SetProperty("device.description", "PACC Output")
	err := sink.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer sink.Close()
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGINT)
	signal.Notify(c, os.Interrupt, syscall.SIGKILL)
	f, err := os.Create("foo.aac")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	enc, err := aac.NewEncoder(f, &aac.Options{
		SampleRate:  48000,
		NumChannels: 2,
	})
	if err != nil {
		log.Fatal(err)
	}
	go readAll(sink, enc)
	<-c
	enc.Close()
}

func readAll(sink *papipes.Sink, enc *aac.Encoder) {

	buf := make([]byte, 4096)
	r := bytes.NewReader(buf)
	for {
		n, err := sink.Read(buf)
		if err != nil {
			fmt.Println("err", err)
			return
		}
		if n < len(buf) {
			continue
			//for i := n; i < len(buf); i++ {
			//buf[i] = 0
			//}
		}
		r.Seek(0, io.SeekStart)
		enc.Encode(r)
	}
}
