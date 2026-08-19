package pulseaudio

import (
	"github.com/jfreymuth/pulse/proto"
)

func (pa *PulseAudioBackend) parsePipeWireSink(s *proto.GetSinkInfoReply, defaultName string) AudioOutput {
	props := cloneProps(s.Properties)
	return AudioOutput{
		Index:       s.SinkIndex,
		Name:        s.SinkName,
		Description: s.Device,
		Nick:        props["node.nick"],
		Muted:       s.Mute,
		Volume:      volumeOf(s.ChannelVolumes),
		State:       sinkStateString(s.State),
		Default:     s.SinkName == defaultName,
		Driver:      s.Driver,
		ActivePort:  s.ActivePortName,
		IsNetwork:   props["node.network"] == "true",
		Props:       props,
	}
}

func (pa *PulseAudioBackend) parsePipeWireSinkInput(s *proto.GetSinkInputInfoReply) AudioClient {
	props := cloneProps(s.Properties)

	return AudioClient{
		ID:      s.SinkInputIndex,
		Name:    clientName(props),
		App:     props["application.name"],
		Muted:   s.Muted,
		Volume:  volumeOf(s.ChannelVolumes),
		Corked:  props["pulse.corked"] == "true",
		Binary:  props["application.process.binary"],
		User:    props["application.process.user"],
		Host:    props["application.process.host"],
		Backend: ServerPipeWire,
		Props:   props,
	}
}
