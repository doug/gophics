package layout

// Axis is a layout direction. The zero value is Vertical: the natural
// default for scrolling and for a zero-value Flex (a column).
type Axis uint8

const (
	Vertical Axis = iota
	Horizontal
)

// MainAlign distributes free space along the main axis when no child flexes.
type MainAlign uint8

const (
	MainStart MainAlign = iota
	MainCenter
	MainEnd
	MainSpaceBetween
)

// CrossAlign positions children across the main axis.
type CrossAlign uint8

const (
	CrossStart CrossAlign = iota
	CrossCenter
	CrossEnd
	CrossStretch
)
