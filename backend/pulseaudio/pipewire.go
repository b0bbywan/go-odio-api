package pulseaudio

import (
	"github.com/the-jonsey/pulseaudio"
)

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
