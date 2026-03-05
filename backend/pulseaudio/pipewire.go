package pulseaudio

import (
	"github.com/the-jonsey/pulseaudio"
)

func (pa *PulseAudioBackend) parsePipeWireSink(s pulseaudio.Sink, defaultName string) AudioOutput {
	props := cloneProps(s.PropList)
	return AudioOutput{
		Index:       s.Index,
		Name:        s.Name,
		Description: s.Description,
		Nick:        props["node.nick"],
		Muted:       s.IsMute(),
		Volume:      s.GetVolume(),
		State:       sinkStateString(s.SinkState),
		Default:     s.Name == defaultName,
		Driver:      s.Driver,
		ActivePort:  s.ActivePortName,
		IsNetwork:   props["node.network"] == "true",
		Props:       props,
	}
}

func (pa *PulseAudioBackend) parsePipeWireSinkInput(s pulseaudio.SinkInput) AudioClient {
	props := cloneProps(s.PropList)

	return AudioClient{
		ID:      s.Index,
		Name:    props["media.name"],
		App:     props["application.name"],
		Muted:   s.IsMute(),
		Volume:  s.GetVolume(),
		Corked:  props["pulse.corked"] == "true",
		Binary:  props["application.process.binary"],
		User:    props["application.process.user"],
		Host:    props["application.process.host"],
		Backend: ServerPipeWire,
		Props:   props,
	}
}
