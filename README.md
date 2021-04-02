# pacc - Pulseaudio Chromecast

A quick-and-dirty WIP hack at routing virtual Pulseaudio output to a Chromecast.

Written in Go, so it's much faster than the equivalent Python `pulseaudio-dlna`

The Python inspiration has a delay of about 30s. 

This project, using FLAC sees ~10s. Using WAV it's down to about 4s.

Some testing around making a little tray icon too.

Right now, `go run . "Name of Chromecast Device"` will start the stream

Email/Twitter at me if you end up using it, but mostly this is just a code dump.
