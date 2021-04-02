package main

import (
	"errors"
	"fmt"
	"os/exec"
)

type NullSink struct {
	Name                    string
	Format                  string
	Rate                    int
	Channels                int
	UseSystemClockForTiming bool
	properties              map[string]interface{}
	open                    bool
	idx                     int
}

func (n *NullSink) Open() error {
	var err error
	if n.Name == "" {
		return errors.New("Name is required")
	}
	args := make([]string, 0)
	args = append(args, "load-module")
	args = append(args, "module-null-sink")
	args = append(args, fmt.Sprintf("sink_name=%s", n.Name))

	if n.Format != "" {
		args = append(args, fmt.Sprintf("format=%s", n.Format))
	}
	if n.Rate > 0 {
		args = append(args, fmt.Sprintf("rate=%d", n.Rate))
	}
	if n.Channels > 0 {
		args = append(args, fmt.Sprintf("channels=%d", n.Channels))
	}

	if n.UseSystemClockForTiming {
		args = append(args, "use_system_clock_for_timing=yes")
	}

	var props string

	for k, v := range n.properties {
		props = props + fmt.Sprintf("%s='%v'", k, v)
	}

	args = append(args, fmt.Sprintf("sink_properties=\"%s\"", props))

	fmt.Println(args)
	cmd := exec.Command("pactl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(string(out))
	}
	if _, err := fmt.Sscanf(string(out), "%d", &n.idx); err != nil {
		return err
	}
	n.open = true
	return nil
}

func (n *NullSink) Close() error {
	args := make([]string, 0)
	args = append(args, "unload-module")
	args = append(args, fmt.Sprintf("%d", n.idx))

	cmd := exec.Command("pactl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(string(out))
	}

	n.open = false

	return nil
}

func (n *NullSink) SetProperty(key string, value interface{}) {
	if n.properties == nil {
		n.properties = make(map[string]interface{})
	}

	n.properties[key] = value
}

func (n *NullSink) GetProperty(key string) interface{} {
	if n.properties == nil {
		return nil
	}

	return n.properties[key]
}
