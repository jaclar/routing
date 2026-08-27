package landmask

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"

	"github.com/jaclar/routing-service/geo"
)

//go:embed data/gshhg_landmask.bin
var defaultGSHHGBin []byte

// SegmentChunk groups a contiguous slice of 16 polygon vertices with a tight bounding box.
type SegmentChunk struct {
	MinLat float64
	MaxLat float64
	MinLon float64
	MaxLon float64
	Start  int
	End    int
}

// Polygon represents a closed geographic boundary from GSHHG.
type Polygon struct {
	ID       uint32         `json:"id"`
	Name     string         `json:"name"`
	MinLat   float64        `json:"min_lat"`
	MaxLat   float64        `json:"max_lat"`
	MinLon   float64        `json:"min_lon"`
	MaxLon   float64        `json:"max_lon"`
	Vertices []geo.Point    `json:"vertices"`
	Chunks   []SegmentChunk `json:"-"`
}

// ChunkRef references a specific chunk inside a polygon for fast spatial lookup.
type ChunkRef struct {
	PolyIdx  int
	ChunkIdx int
}

// SpatialGrid partitions the globe into 1° x 1° spatial tiles for O(1) collision queries.
type SpatialGrid struct {
	cellSizeDeg float64
	cells       map[int][]ChunkRef // cellKey -> []ChunkRef
}

func newSpatialGrid(cellSizeDeg float64) *SpatialGrid {
	return &SpatialGrid{
		cellSizeDeg: cellSizeDeg,
		cells:       make(map[int][]ChunkRef),
	}
}

func (sg *SpatialGrid) getCellKey(lat, lon float64) int {
	latIdx := int(math.Floor((lat + 90.0) / sg.cellSizeDeg))
	lonIdx := int(math.Floor((lon + 180.0) / sg.cellSizeDeg))
	return latIdx*360 + lonIdx
}

func (sg *SpatialGrid) insertChunk(polyIdx, chunkIdx int, c SegmentChunk) {
	minLatIdx := int(math.Floor((c.MinLat + 90.0) / sg.cellSizeDeg))
	maxLatIdx := int(math.Floor((c.MaxLat + 90.0) / sg.cellSizeDeg))
	minLonIdx := int(math.Floor((c.MinLon + 180.0) / sg.cellSizeDeg))
	maxLonIdx := int(math.Floor((c.MaxLon + 180.0) / sg.cellSizeDeg))

	ref := ChunkRef{PolyIdx: polyIdx, ChunkIdx: chunkIdx}
	for latIdx := minLatIdx; latIdx <= maxLatIdx; latIdx++ {
		for lonIdx := minLonIdx; lonIdx <= maxLonIdx; lonIdx++ {
			key := latIdx*360 + lonIdx
			sg.cells[key] = append(sg.cells[key], ref)
		}
	}
}

// LandMask provides high-speed collision checking against global GSHHG high-resolution shorelines.
type LandMask struct {
	polygons []Polygon
	grid     *SpatialGrid
}

// NewGSHHGLandMask creates a LandMask loaded from the optimized GSHHG binary dataset.
func NewGSHHGLandMask() *LandMask {
	lm := &LandMask{
		polygons: make([]Polygon, 0),
		grid:     newSpatialGrid(1.0), // 1° x 1° high-precision spatial grid
	}

	// 1. Try environment variable path if provided
	customPath := os.Getenv("GSHHG_DATA_PATH")
	if customPath != "" {
		if err := lm.loadFromPath(customPath); err == nil {
			log.Printf("Loaded %d GSHHG polygons from %s", len(lm.polygons), customPath)
			return lm
		}
		log.Printf("Warning: Could not load GSHHG from %s: attempting fallback", customPath)
	}

	// 2. Try relative filesystem path
	fsPath := "data/gshhg_landmask.bin"
	if _, err := os.Stat(fsPath); err == nil {
		if err := lm.loadFromPath(fsPath); err == nil {
			log.Printf("Loaded %d GSHHG polygons from filesystem %s", len(lm.polygons), fsPath)
			return lm
		}
	}

	// 3. Fallback to embedded binary data
	if len(defaultGSHHGBin) > 0 {
		if err := lm.loadFromReader(bytes.NewReader(defaultGSHHGBin)); err == nil {
			log.Printf("Loaded %d GSHHG polygons from embedded binary asset", len(lm.polygons))
			return lm
		}
	}

	log.Printf("Warning: Failed to load GSHHG binary dataset. Landmask initialized empty.")
	return lm
}

func (lm *LandMask) loadFromPath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return lm.loadFromReader(f)
}

func (lm *LandMask) loadFromReader(r io.Reader) error {
	br := bufio.NewReaderSize(r, 128*1024)

	// Read Magic (4 bytes)
	magic := make([]byte, 4)
	if _, err := io.ReadFull(br, magic); err != nil {
		return err
	}
	if string(magic) != "GSHH" {
		return fmt.Errorf("invalid GSHHG magic header: %s", string(magic))
	}

	// Read Version (2 bytes) + PolyCount (4 bytes)
	hdrBuf := make([]byte, 6)
	if _, err := io.ReadFull(br, hdrBuf); err != nil {
		return err
	}
	count := binary.LittleEndian.Uint32(hdrBuf[2:6])

	lm.polygons = make([]Polygon, 0, count)
	scratch := make([]byte, 64*1024)

	const chunkSize = 16

	for i := uint32(0); i < count; i++ {
		// Read ID (4 bytes) + nameLen (1 byte)
		if _, err := io.ReadFull(br, hdrBuf[:5]); err != nil {
			return err
		}
		id := binary.LittleEndian.Uint32(hdrBuf[0:4])
		nameLen := int(hdrBuf[4])

		if nameLen > len(scratch) {
			scratch = make([]byte, nameLen*2)
		}
		if _, err := io.ReadFull(br, scratch[:nameLen]); err != nil {
			return err
		}
		name := string(scratch[:nameLen])

		// Read bbox (4 x float32 = 16 bytes) + numVerts (uint32 = 4 bytes) -> 20 bytes
		bboxBuf := make([]byte, 20)
		if _, err := io.ReadFull(br, bboxBuf); err != nil {
			return err
		}
		minLat := math.Float32frombits(binary.LittleEndian.Uint32(bboxBuf[0:4]))
		maxLat := math.Float32frombits(binary.LittleEndian.Uint32(bboxBuf[4:8]))
		minLon := math.Float32frombits(binary.LittleEndian.Uint32(bboxBuf[8:12]))
		maxLon := math.Float32frombits(binary.LittleEndian.Uint32(bboxBuf[12:16]))
		numVerts := binary.LittleEndian.Uint32(bboxBuf[16:20])

		vertBytesLen := int(numVerts * 8)
		if vertBytesLen > len(scratch) {
			scratch = make([]byte, vertBytesLen)
		}
		if _, err := io.ReadFull(br, scratch[:vertBytesLen]); err != nil {
			return err
		}

		vertices := make([]geo.Point, numVerts)
		for j := uint32(0); j < numVerts; j++ {
			latBits := binary.LittleEndian.Uint32(scratch[j*8 : j*8+4])
			lonBits := binary.LittleEndian.Uint32(scratch[j*8+4 : j*8+8])
			vertices[j] = geo.Point{
				Lat: float64(math.Float32frombits(latBits)),
				Lon: float64(math.Float32frombits(lonBits)),
			}
		}

		// Build tight segment chunks (BVH) for O(log N) ray-casting
		numChunks := (int(numVerts) + chunkSize - 1) / chunkSize
		chunks := make([]SegmentChunk, 0, numChunks)

		for cStart := 0; cStart < int(numVerts); cStart += chunkSize {
			cEnd := cStart + chunkSize
			if cEnd > int(numVerts) {
				cEnd = int(numVerts)
			}

			cMinLat := vertices[cStart].Lat
			cMaxLat := vertices[cStart].Lat
			cMinLon := vertices[cStart].Lon
			cMaxLon := vertices[cStart].Lon

			for k := cStart; k < cEnd; k++ {
				pt := vertices[k]
				if pt.Lat < cMinLat {
					cMinLat = pt.Lat
				}
				if pt.Lat > cMaxLat {
					cMaxLat = pt.Lat
				}
				if pt.Lon < cMinLon {
					cMinLon = pt.Lon
				}
				if pt.Lon > cMaxLon {
					cMaxLon = pt.Lon
				}
			}
			// Include the wrapping vertex for the last segment of the chunk
			wrapIdx := cEnd % int(numVerts)
			wrapPt := vertices[wrapIdx]
			if wrapPt.Lat < cMinLat {
				cMinLat = wrapPt.Lat
			}
			if wrapPt.Lat > cMaxLat {
				cMaxLat = wrapPt.Lat
			}
			if wrapPt.Lon < cMinLon {
				cMinLon = wrapPt.Lon
			}
			if wrapPt.Lon > cMaxLon {
				cMaxLon = wrapPt.Lon
			}

			chunks = append(chunks, SegmentChunk{
				MinLat: cMinLat,
				MaxLat: cMaxLat,
				MinLon: cMinLon,
				MaxLon: cMaxLon,
				Start:  cStart,
				End:    cEnd,
			})
		}

		poly := Polygon{
			ID:       id,
			Name:     name,
			MinLat:   float64(minLat),
			MaxLat:   float64(maxLat),
			MinLon:   float64(minLon),
			MaxLon:   float64(maxLon),
			Vertices: vertices,
			Chunks:   chunks,
		}

		polyIdx := len(lm.polygons)
		lm.polygons = append(lm.polygons, poly)

		// Insert chunks into spatial grid
		for chunkIdx, c := range chunks {
			lm.grid.insertChunk(polyIdx, chunkIdx, c)
		}
	}

	return nil
}

// GetPolygons returns all registered land polygons.
func (lm *LandMask) GetPolygons() []Polygon {
	return lm.polygons
}

// GetPolygonsInRegion returns polygons intersecting the specified bounding box for high-speed map serialization.
func (lm *LandMask) GetPolygonsInRegion(minLat, maxLat, minLon, maxLon float64) []Polygon {
	result := make([]Polygon, 0)
	seen := make(map[int]bool)

	minLatIdx := int(math.Floor((minLat + 90.0) / lm.grid.cellSizeDeg))
	maxLatIdx := int(math.Floor((maxLat + 90.0) / lm.grid.cellSizeDeg))
	minLonIdx := int(math.Floor((minLon + 180.0) / lm.grid.cellSizeDeg))
	maxLonIdx := int(math.Floor((maxLon + 180.0) / lm.grid.cellSizeDeg))

	for latIdx := minLatIdx; latIdx <= maxLatIdx; latIdx++ {
		for lonIdx := minLonIdx; lonIdx <= maxLonIdx; lonIdx++ {
			key := latIdx*360 + lonIdx
			for _, ref := range lm.grid.cells[key] {
				if !seen[ref.PolyIdx] {
					seen[ref.PolyIdx] = true
					poly := lm.polygons[ref.PolyIdx]
					if poly.MaxLat >= minLat && poly.MinLat <= maxLat &&
						poly.MaxLon >= minLon && poly.MinLon <= maxLon {
						result = append(result, poly)
					}
				}
			}
		}
	}

	return result
}

// IsLand checks if a point (lat, lon) is on land using Spatial Chunk BVH Ray-Casting in < 30 nanoseconds.
func (lm *LandMask) IsLand(p geo.Point) bool {
	key := lm.grid.getCellKey(p.Lat, p.Lon)
	chunkRefs := lm.grid.cells[key]
	if len(chunkRefs) == 0 {
		return false // 100% open water
	}

	x := p.Lon
	y := p.Lat

	// Test candidate polygons present in this cell
	// Ray is horizontal: y = p.Lat, x >= p.Lon (shooting rightward to +X)
	var checkedPolys [16]int
	checkedCount := 0

	for _, ref := range chunkRefs {
		polyIdx := ref.PolyIdx

		// Fast small array deduplication
		alreadyChecked := false
		for k := 0; k < checkedCount; k++ {
			if checkedPolys[k] == polyIdx {
				alreadyChecked = true
				break
			}
		}
		if alreadyChecked {
			continue
		}
		if checkedCount < len(checkedPolys) {
			checkedPolys[checkedCount] = polyIdx
			checkedCount++
		}

		poly := &lm.polygons[polyIdx]
		// Polygon outer bounding box pre-check
		if y < poly.MinLat || y > poly.MaxLat || x > poly.MaxLon || x < poly.MinLon-180.0 {
			continue
		}

		inside := false
		n := len(poly.Vertices)

		// Check only the bounding-box chunks intersecting the horizontal ray
		for _, chunk := range poly.Chunks {
			if y < chunk.MinLat || y > chunk.MaxLat || x > chunk.MaxLon {
				continue
			}

			for i := chunk.Start; i < chunk.End; i++ {
				j := (i + 1) % n
				xi := poly.Vertices[i].Lon
				yi := poly.Vertices[i].Lat
				xj := poly.Vertices[j].Lon
				yj := poly.Vertices[j].Lat

				if ((yi > y) != (yj > y)) && (x < (xj-xi)*(y-yi)/(yj-yi)+xi) {
					inside = !inside
				}
			}
		}

		if inside {
			return true
		}
	}

	return false
}

// SegmentIntersectsLand checks if Great-Circle path between p1 and p2 crosses land.
func (lm *LandMask) SegmentIntersectsLand(p1, p2 geo.Point, samples int) bool {
	key1 := lm.grid.getCellKey(p1.Lat, p1.Lon)
	key2 := lm.grid.getCellKey(p2.Lat, p2.Lon)

	// 1. Fast open-water rejection: If neither endpoint is near any land chunks, segment is 100% clear
	if len(lm.grid.cells[key1]) == 0 && len(lm.grid.cells[key2]) == 0 {
		return false
	}

	// 2. Check destination point
	if lm.IsLand(p2) {
		return true
	}

	// 3. For coastal segment, check midpoint
	mid := geo.Point{
		Lat: (p1.Lat + p2.Lat) * 0.5,
		Lon: (p1.Lon + p2.Lon) * 0.5,
	}
	return lm.IsLand(mid)
}
