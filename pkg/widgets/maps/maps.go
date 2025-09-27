package maps

import (
	"image"
	"math"
	"net/http"
	"net/url"

	"github.com/nfnt/resize"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"golang.org/x/image/draw"
)

const tileSize = 256

// Map widget renders an interactive map using OpenStreetMap tile data.
type Map struct {
	widget.BaseWidget

	pixels     *image.NRGBA
	w, h       int
	zoom, x, y int
	offX, offY int

	cl *http.Client

	tileSource       string // url to download xyz tiles (example: "https://tile.openstreetmap.org/%d/%d/%d.png")
	hideAttribution  bool   // enable copyright attribution
	attributionLabel string // label for attribution (example: "OpenStreetMap")
	attributionURL   string // url for attribution (example: "https://openstreetmap.org")
	hideZoomButtons  bool   // enable zoom buttons
	hideMoveButtons  bool   // enable move map buttons
}

// MapOption configures the provided map with different features.
type MapOption func(*Map)

// WithOsmTiles configures the map to use osm tile source.
func WithOsmTiles() MapOption {
	return func(m *Map) {
		m.tileSource = "https://tile.openstreetmap.org/%d/%d/%d.png"
		m.attributionLabel = "OpenStreetMap"
		m.attributionURL = "https://openstreetmap.org"
		m.hideAttribution = false
	}
}

// WithTileSource configures the map to use a custom tile source.
func WithTileSource(tileSource string) MapOption {
	return func(m *Map) {
		m.tileSource = tileSource
	}
}

// WithAttribution configures the map widget to display an attribution.
func WithAttribution(enable bool, label, url string) MapOption {
	return func(m *Map) {
		m.hideAttribution = !enable
		m.attributionLabel = label
		m.attributionURL = url
	}
}

// WithZoomButtons enables or disables zoom controls.
func WithZoomButtons(enable bool) MapOption {
	return func(m *Map) {
		m.hideZoomButtons = !enable
	}
}

// WithScrollButtons enables or disables map scroll controls.
func WithScrollButtons(enable bool) MapOption {
	return func(m *Map) {
		m.hideMoveButtons = !enable
	}
}

// WithHTTPClient configures the map to use a custom http client.
func WithHTTPClient(client *http.Client) MapOption {
	return func(m *Map) {
		m.cl = client
	}
}

// NewMap creates a new instance of the map widget.
func NewMap() *Map {
	m := &Map{cl: &http.Client{}}
	WithOsmTiles()(m)
	m.ExtendBaseWidget(m)
	return m
}

// NewMapWithOptions creates a new instance of the map widget with provided map options.
func NewMapWithOptions(opts ...MapOption) *Map {
	m := NewMap()
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// MinSize returns the smallest possible size for a widget.
// For our map this is a constant size representing a single tile on a device with
// the highest known DPI (4x).
func (m *Map) MinSize() fyne.Size {
	return fyne.NewSize(64, 64)
}

// PanEast will move the map to the East by 1 tile.
func (m *Map) PanEast() {
	m.x++
	m.Refresh()
}

// PanNorth will move the map to the North by 1 tile.
func (m *Map) PanNorth() {
	m.y--
	m.Refresh()
}

// PanSouth will move the map to the South by 1 tile.
func (m *Map) PanSouth() {
	m.y++
	m.Refresh()
}

// PanWest will move the map to the west by 1 tile.
func (m *Map) PanWest() {
	m.x--
	m.Refresh()
}

// Zoom sets the zoom level to a specific value, between 0 and 19.
func (m *Map) Zoom(zoom int) {
	if zoom < 0 || zoom > 19 {
		return
	}
	delta := zoom - m.zoom
	if delta > 0 {
		for i := 0; i < delta; i++ {
			m.zoomInStep()
		}
	} else if delta < 0 {
		for i := 0; i > delta; i-- {
			m.zoomOutStep()
		}
	}
	m.Refresh()
}

// ZoomIn steps the scale of this map to be one step zoomed in.
func (m *Map) ZoomIn() {
	if m.zoom >= 19 {
		return
	}
	m.zoomInStep()
	m.Refresh()
}

// ZoomOut steps the scale of this map to be one step zoomed out.
func (m *Map) ZoomOut() {
	if m.zoom <= 0 {
		return
	}
	m.zoomOutStep()
	m.Refresh()
}

// CreateRenderer returns the renderer for this widget.
// A map renderer is simply the map Raster with user interface elements overlaid.
func (m *Map) CreateRenderer() fyne.WidgetRenderer {
	var zoom fyne.CanvasObject
	if !m.hideZoomButtons {
		zoom = container.NewVBox(
			newMapButton(theme.ZoomInIcon(), m.ZoomIn),
			newMapButton(theme.ZoomOutIcon(), m.ZoomOut))
	}

	var move fyne.CanvasObject
	if !m.hideMoveButtons {
		buttonLayout := container.NewGridWithColumns(3, layout.NewSpacer(),
			newMapButton(theme.MoveUpIcon(), m.PanNorth), layout.NewSpacer(),
			newMapButton(theme.NavigateBackIcon(), m.PanWest), layout.NewSpacer(),
			newMapButton(theme.NavigateNextIcon(), m.PanEast), layout.NewSpacer(),
			newMapButton(theme.MoveDownIcon(), m.PanSouth), layout.NewSpacer())
		move = container.NewVBox(buttonLayout)
	}

	var copyright fyne.CanvasObject
	if !m.hideAttribution {
		license, _ := url.Parse(m.attributionURL)
		link := widget.NewHyperlink(m.attributionLabel, license)
		copyright = container.NewHBox(layout.NewSpacer(), link)
	}

	overlay := container.NewBorder(nil, copyright, move, zoom)

	c := container.NewStack(canvas.NewRaster(m.draw), container.NewPadded(overlay))
	return widget.NewSimpleRenderer(c)
}

func (m *Map) draw(w, h int) image.Image {
	scale := 1
	tileSize := tileSize
	// TODO use retina tiles once OSM supports it in their server (text scaling issues)...
	if c := fyne.CurrentApp().Driver().CanvasForObject(m); c != nil {
		scale = int(c.Scale())
		if scale < 1 {
			scale = 1
		}
		tileSize = tileSize * scale
	}

	if m.w != w || m.h != h {
		m.pixels = image.NewNRGBA(image.Rect(0, 0, w, h))
	}

	midTileX := (w - tileSize*2) / 2
	midTileY := (h - tileSize*2) / 2
	if m.zoom == 0 {
		midTileX += tileSize / 2
		midTileY += tileSize / 2
	}

	midTileX += (tileSize / 2) - m.offX
	midTileY += (tileSize / 2) - m.offY

	count := 1 << m.zoom
	mx := m.x + int(float32(count)/2-0.5)
	my := m.y + int(float32(count)/2-0.5)
	firstTileX := mx - int(math.Ceil(float64(midTileX)/float64(tileSize)))
	firstTileY := my - int(math.Ceil(float64(midTileY)/float64(tileSize)))
	for x := firstTileX; (x-firstTileX)*tileSize <= w+tileSize; x++ {
		for y := firstTileY; (y-firstTileY)*tileSize <= h+tileSize; y++ {
			if x < 0 || y < 0 || x >= int(count) || y >= int(count) {
				continue
			}

			src, err := getTile(m.tileSource, x, y, m.zoom, m.cl)
			if err != nil {
				fyne.LogError("tile fetch error", err)
				continue
			}

			pos := image.Pt(midTileX+(x-mx)*tileSize,
				midTileY+(y-my)*tileSize)
			scaled := src
			if scale > 1 {
				scaled = resize.Resize(uint(tileSize), uint(tileSize), src, resize.Lanczos2)
			}
			draw.Copy(m.pixels, pos, scaled, image.Rect(0, 0, tileSize, tileSize), draw.Over, nil)
		}
	}

	return m.pixels
}

func (m *Map) zoomInStep() {
	m.zoom++
	m.x *= 2
	m.y *= 2
	// NEW: scale offsets
	m.offX *= 2
	m.offY *= 2
	// keep them in 0..255
	m.offX &= 255
	m.offY &= 255
}

func (m *Map) zoomOutStep() {
	m.zoom--
	m.x /= 2
	m.y /= 2
	// NEW: scale offsets (floor)
	m.offX /= 2
	m.offY /= 2
}

func (m *Map) SetCenter(lat, lon float64) {
	tileX, tileY, _, _, offX, offY := SlippyTile(lat, lon, m.zoom)

	n := 1 << m.zoom
	centerOffset := 0
	if m.zoom >= 1 {
		centerOffset = (n / 2) - 1
	}

	// Store "relative" center tile plus its pixel offsets
	m.x = tileX - centerOffset
	m.y = tileY - centerOffset
	m.offX = offX
	m.offY = offY

	m.Refresh()
}

// SlippyTile converts WGS84 latitude/longitude to OSM/Web-Mercator
// slippy map indices at the given zoom.
// Returns:
//
//	tileX, tileY      - integer tile coordinates (0..2^z-1)
//	pixelX, pixelY    - global pixel coordinates from the map origin (0,0)
//	offsetX, offsetY  - pixel offset within the tile (0..255)
func SlippyTile(lat, lon float64, zoom int) (tileX, tileY, pixelX, pixelY, offsetX, offsetY int) {
	// Clamp latitude to Web Mercator's valid range to avoid infinities.
	const maxLat = 85.05112878
	if lat > maxLat {
		lat = maxLat
	} else if lat < -maxLat {
		lat = -maxLat
	}

	// Normalize longitude to [-180, 180)
	lon = math.Mod(lon+180.0, 360.0)
	if lon < 0 {
		lon += 360.0
	}
	lon -= 180.0

	// Number of tiles at this zoom
	if zoom < 0 {
		zoom = 0
	}
	n := float64(uint64(1) << uint(zoom)) // 2^zoom

	// Convert to tile coordinates (floating)
	x := (lon + 180.0) / 360.0 * n

	latRad := lat * math.Pi / 180.0
	y := (1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n

	// Integer tile indices
	tileX = int(math.Floor(x))
	tileY = int(math.Floor(y))

	// Clamp to valid range (handles edge cases right at 180° / -180°)
	maxIdx := int(n) - 1
	if tileX < 0 {
		tileX = 0
	} else if tileX > maxIdx {
		tileX = maxIdx
	}
	if tileY < 0 {
		tileY = 0
	} else if tileY > maxIdx {
		tileY = maxIdx
	}

	// Global pixel coordinates
	pixelX = int(math.Floor(x*256.0 + 0.0))
	pixelY = int(math.Floor(y*256.0 + 0.0))

	// Pixel offset inside the tile
	offsetX = pixelX % 256
	offsetY = pixelY % 256
	if offsetX < 0 {
		offsetX += 256
	}
	if offsetY < 0 {
		offsetY += 256
	}
	return
}
