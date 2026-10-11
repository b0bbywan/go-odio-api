package input

type Kind string

const (
	KindIR     Kind = "ir"
	KindGPIO   Kind = "gpio"
	KindForced Kind = "forced"
)
