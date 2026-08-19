package pulseaudio

import (
	"fmt"
	"math"
	"os"
	"path"
	"time"

	"github.com/b0bbywan/go-odio-api/logger"
	"github.com/jfreymuth/pulse/proto"
)

const requestTimeout = 5 * time.Second

// connect opens the native protocol connection and registers the client name.
func (pa *PulseAudioBackend) connect() error {
	client, conn, err := proto.Connect(pa.address)
	if err != nil {
		return err
	}
	client.SetTimeout(requestTimeout)

	props := proto.PropList{
		"application.name":           proto.PropListString(path.Base(os.Args[0])),
		"application.process.id":     proto.PropListString(fmt.Sprintf("%d", os.Getpid())),
		"application.process.binary": proto.PropListString(os.Args[0]),
	}
	if hostname, err := os.Hostname(); err == nil {
		props["application.process.host"] = proto.PropListString(hostname)
	}
	if err := client.Request(&proto.SetClientName{Props: props}, &proto.SetClientNameReply{}); err != nil {
		if cerr := conn.Close(); cerr != nil {
			logger.Warn("[pulseaudio] failed to close connection: %v", cerr)
		}
		return err
	}

	pa.client = client
	pa.conn = conn
	pa.connected.Store(true)
	return nil
}

func (pa *PulseAudioBackend) serverInfo() (*proto.GetServerInfoReply, error) {
	var reply proto.GetServerInfoReply
	if err := pa.client.Request(&proto.GetServerInfo{}, &reply); err != nil {
		return nil, err
	}
	return &reply, nil
}

func (pa *PulseAudioBackend) sinks() ([]*proto.GetSinkInfoReply, error) {
	var reply proto.GetSinkInfoListReply
	if err := pa.client.Request(&proto.GetSinkInfoList{}, &reply); err != nil {
		return nil, err
	}
	return reply, nil
}

func (pa *PulseAudioBackend) sinkInputs() ([]*proto.GetSinkInputInfoReply, error) {
	var reply proto.GetSinkInputInfoListReply
	if err := pa.client.Request(&proto.GetSinkInputInfoList{}, &reply); err != nil {
		return nil, err
	}
	return reply, nil
}

func (pa *PulseAudioBackend) sources() ([]*proto.GetSourceInfoReply, error) {
	var reply proto.GetSourceInfoListReply
	if err := pa.client.Request(&proto.GetSourceInfoList{}, &reply); err != nil {
		return nil, err
	}
	return reply, nil
}

func (pa *PulseAudioBackend) modules() ([]*proto.GetModuleInfoReply, error) {
	var reply proto.GetModuleInfoListReply
	if err := pa.client.Request(&proto.GetModuleInfoList{}, &reply); err != nil {
		return nil, err
	}
	return reply, nil
}

func (pa *PulseAudioBackend) setSinkVolume(s *proto.GetSinkInfoReply, vol float32) error {
	return pa.client.Request(&proto.SetSinkVolume{
		SinkIndex:      proto.Undefined,
		SinkName:       s.SinkName,
		ChannelVolumes: channelVolumes(len(s.ChannelVolumes), vol),
	}, nil)
}

func (pa *PulseAudioBackend) setSinkMute(s *proto.GetSinkInfoReply, mute bool) error {
	return pa.client.Request(&proto.SetSinkMute{SinkIndex: proto.Undefined, SinkName: s.SinkName, Mute: mute}, nil)
}

func (pa *PulseAudioBackend) setSinkInputVolume(s *proto.GetSinkInputInfoReply, vol float32) error {
	return pa.client.Request(&proto.SetSinkInputVolume{
		SinkInputIndex: s.SinkInputIndex,
		ChannelVolumes: channelVolumes(len(s.ChannelVolumes), vol),
	}, nil)
}

func (pa *PulseAudioBackend) setSinkInputMute(s *proto.GetSinkInputInfoReply, mute bool) error {
	return pa.client.Request(&proto.SetSinkInputMute{SinkInputIndex: s.SinkInputIndex, Mute: mute}, nil)
}

func (pa *PulseAudioBackend) setDefaultSink(name string) error {
	return pa.client.Request(&proto.SetDefaultSink{SinkName: name}, nil)
}

// volumeOf reports the first channel's volume as a fraction of normal volume,
// rounded to two decimals.
func volumeOf(cv proto.ChannelVolumes) float32 {
	if len(cv) == 0 {
		return 0
	}
	return float32(math.Round(float64(cv[0])/float64(proto.VolumeNorm)*100)) / 100
}

// channelVolumes spreads vol evenly over n channels, matching the object's
// channel count as the server requires.
func channelVolumes(n int, vol float32) proto.ChannelVolumes {
	if n == 0 {
		n = 1
	}
	cv := make(proto.ChannelVolumes, n)
	for i := range cv {
		cv[i] = proto.Volume(vol * float32(proto.VolumeNorm))
	}
	return cv
}
