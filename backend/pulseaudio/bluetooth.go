package pulseaudio

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/b0bbywan/go-odio-api/logger"
	"github.com/jfreymuth/pulse/proto"
)

// preferredBluetoothCodecs lists A2DP codecs by preference; the first one the
// server offers is forced on the device.
var preferredBluetoothCodecs = []string{"aptx_hd", "aptx"}

// ensureBluetoothCodec switches the device behind src to the best preferred
// codec when a better one than the active one may be available. Each device
// is only attempted once: the switch makes the remote reconnect, and a device
// that renegotiates the old codec must not be bounced again.
func (pa *PulseAudioBackend) ensureBluetoothCodec(src *proto.GetSourceInfoReply) {
	current := src.Properties["bluetooth.codec"].String()
	if current == preferredBluetoothCodecs[0] {
		return
	}

	if _, seen := pa.btCodecAttempted.LoadOrStore(bluetoothAddress(src.SourceName), struct{}{}); seen {
		return
	}

	go pa.switchBluetoothCodec(src.CardIndex, current)
}

func (pa *PulseAudioBackend) switchBluetoothCodec(cardIndex uint32, current string) {
	card, err := pa.findCard(cardIndex)
	if err != nil {
		logger.Warn("[pulseaudio] bluetooth codec: %v", err)
		return
	}

	codecs, err := pa.listBluetoothCodecs(card.CardName)
	if err != nil {
		logger.Warn("[pulseaudio] bluetooth codec: %v", err)
		return
	}
	target := pickBluetoothCodec(codecs, current)
	if target == "" {
		logger.Debug("[pulseaudio] %s stays on %s (available: %s)", card.CardName, current, strings.Join(codecs, ", "))
		return
	}

	if _, err := pa.sendCardMessage(card.CardName, "switch-codec", fmt.Sprintf("%q", target)); err != nil {
		logger.Warn("[pulseaudio] bluetooth codec: %v", err)
		return
	}
	logger.Info("[pulseaudio] switched %s from %s to %s", card.CardName, current, target)
}

// bluetoothAddress extracts the device address from a bluez source name
// ("bluez_source.<ADDR>.<profile>").
func bluetoothAddress(sourceName string) string {
	addr := strings.TrimPrefix(sourceName, "bluez_source.")
	if i := strings.Index(addr, "."); i >= 0 {
		addr = addr[:i]
	}
	return addr
}

// pickBluetoothCodec returns the first preferred codec the device offers, or
// "" when none beats the current one.
func pickBluetoothCodec(available []string, current string) string {
	for _, codec := range preferredBluetoothCodecs {
		if codec == current {
			return ""
		}
		if slices.Contains(available, codec) {
			return codec
		}
	}
	return ""
}

func (pa *PulseAudioBackend) listBluetoothCodecs(cardName string) ([]string, error) {
	out, err := pa.sendCardMessage(cardName, "list-codecs", "")
	if err != nil {
		return nil, err
	}
	return parseBluetoothCodecs(out)
}

// parseBluetoothCodecs decodes the list-codecs reply, a JSON array of
// {"name", "description"} objects.
func parseBluetoothCodecs(out string) ([]string, error) {
	var codecs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &codecs); err != nil {
		return nil, fmt.Errorf("failed to parse codec list %q: %w", out, err)
	}
	names := make([]string, 0, len(codecs))
	for _, c := range codecs {
		names = append(names, c.Name)
	}
	return names, nil
}

// sendCardMessage sends a message to the card's bluez handler
// (/card/<name>/bluez) and returns the raw response.
func (pa *PulseAudioBackend) sendCardMessage(cardName, message, param string) (string, error) {
	var reply proto.SendObjectMessageReply
	err := pa.client.Request(&proto.SendObjectMessage{
		ObjectPath: fmt.Sprintf("/card/%s/bluez", cardName),
		Message:    message,
		Parameters: param,
	}, &reply)
	if err != nil {
		return "", fmt.Errorf("%s %s on %s failed: %w", message, param, cardName, err)
	}
	return reply.Response, nil
}

func (pa *PulseAudioBackend) findCard(index uint32) (*proto.GetCardInfoReply, error) {
	var cards proto.GetCardInfoListReply
	if err := pa.client.Request(&proto.GetCardInfoList{}, &cards); err != nil {
		return nil, err
	}
	for _, c := range cards {
		if c.CardIndex == index {
			return c, nil
		}
	}
	return nil, &NotFoundError{Resource: "card", Name: fmt.Sprintf("%d", index)}
}
