package input

type Kind string

const (
	KindIR     Kind = "ir"
	KindCEC    Kind = "cec"
	KindGPIO   Kind = "gpio"
	KindForced Kind = "forced"
)
