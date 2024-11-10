package widgets

// ColorScheme defines the type of color scale to use
type ColorScheme int

const (
	TraditionalScale ColorScheme = iota // Green to Red
	BlueYellowScale                     // Colorblind-friendly scale
)
