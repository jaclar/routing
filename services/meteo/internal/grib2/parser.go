package grib2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/shyrmapp/aec"
	"sailboat/meteo/internal/model"
)

var (
	ErrInvalidMagic   = errors.New("invalid GRIB2 magic identifier")
	ErrUnsupportedVer = errors.New("unsupported GRIB edition")
	ErrTruncatedMsg   = errors.New("truncated GRIB message")
)

// Message encapsulates a decoded single-field GRIB2 record.
type Message struct {
	Discipline    uint8
	ReferenceTime time.Time
	ValidTime     time.Time
	StepHours     int
	ParamCategory uint8
	ParamNumber   uint8
	SurfaceType   uint8
	SurfaceValue  float64

	// Grid Definition
	GridTemplate uint16
	Ni           int
	Nj           int
	La1, Lo1     float64
	La2, Lo2     float64
	Di, Dj       float64
	ScanningMode uint8

	// Data
	DataPoints int
	Values     []float32
}

type ccsdsMeta struct {
	flags     uint32
	blockSize int
	rsi       int
}

type complexPackingMeta struct {
	refVal                   float32
	binScale                 int16
	decScale                 int16
	numBits                  uint8
	originalType             uint8
	groupSplitting           uint8
	missingValMgmt           uint8
	primaryMissing           uint32
	secondaryMissing         uint32
	numGroups                int
	refGroupWidths           uint8
	numBitsGroupWidths       uint8
	refGroupLengths          int
	lenIncrementGroupLengths int
	trueLenLastGroup         int
	numBitsGroupLengths      uint8
	orderSpatialDiff         uint8
	numOctetsExtraDescriptor uint8
}

// Parse decodes a raw GRIB2 byte sequence into a Message.
func Parse(data []byte) (*Message, error) {
	if len(data) < 16 {
		return nil, ErrTruncatedMsg
	}

	// Section 0: Indicator (16 bytes)
	if string(data[0:4]) != "GRIB" {
		// Attempt to search for GRIB header if data has a leading offset
		idx := bytes.Index(data, []byte("GRIB"))
		if idx == -1 || len(data)-idx < 16 {
			return nil, ErrInvalidMagic
		}
		data = data[idx:]
	}

	discipline := data[6]
	edition := data[7]
	if edition != 2 {
		return nil, fmt.Errorf("%w: edition %d", ErrUnsupportedVer, edition)
	}

	totalLen := binary.BigEndian.Uint64(data[8:16])
	if uint64(len(data)) < totalLen && len(data) < 1000 {
		return nil, fmt.Errorf("truncated GRIB message: expected %d bytes, got %d", totalLen, len(data))
	}

	msg := &Message{
		Discipline: discipline,
	}

	offset := 16
	var (
		refTime time.Time
		stepHrs int
		ni, nj  int
		la1, lo1 float64
		la2, lo2 float64
		di, dj   float64
		scanMode uint8

		paramCat uint8
		paramNum uint8
		surfType uint8
		surfVal  float64

		repTemplate uint16
		refVal      float32
		binScale    int16
		decScale    int16
		numBits     uint8
		sec3Points  int
		sec5Points  int
		cpm         complexPackingMeta
		ccsdsOpt    ccsdsMeta

		hasBitmap bool
		bitmap    []byte
	)

	for offset < len(data) {
		// Check for end section "7777"
		if offset+4 <= len(data) && string(data[offset:offset+4]) == "7777" {
			break
		}

		if offset+5 > len(data) {
			break
		}

		secLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if secLen <= 0 || offset+secLen > len(data) {
			break
		}

		secNum := data[offset+4]
		secData := data[offset : offset+secLen]

		switch secNum {
		case 1: // Identification Section
			if len(secData) >= 21 {
				year := binary.BigEndian.Uint16(secData[12:14])
				month := int(secData[14])
				day := int(secData[15])
				hour := int(secData[16])
				min := int(secData[17])
				sec := int(secData[18])
				refTime = time.Date(int(year), time.Month(month), day, hour, min, sec, 0, time.UTC)
				msg.ReferenceTime = refTime
			}

		case 3: // Grid Definition Section
			if len(secData) >= 10 {
				sec3Points = int(binary.BigEndian.Uint32(secData[6:10]))
			}
			if len(secData) >= 14 {
				gridTemplate := binary.BigEndian.Uint16(secData[12:14])
				msg.GridTemplate = gridTemplate
				if gridTemplate == 0 || gridTemplate == 40 { // Lat/Lon Equirectangular grid
					if len(secData) >= 72 {
						ni = int(binary.BigEndian.Uint32(secData[30:34]))
						nj = int(binary.BigEndian.Uint32(secData[34:38]))
						la1 = float64(readGRIBSigned(secData[46:50])) * 1e-6
						lo1 = float64(readGRIBSigned(secData[50:54])) * 1e-6
						la2 = float64(readGRIBSigned(secData[55:59])) * 1e-6
						lo2 = float64(readGRIBSigned(secData[59:63])) * 1e-6
						di = float64(binary.BigEndian.Uint32(secData[63:67])) * 1e-6
						dj = float64(binary.BigEndian.Uint32(secData[67:71])) * 1e-6
						scanMode = secData[71]
					}
				} else if gridTemplate == 100 || gridTemplate == 101 { // General Unstructured (e.g. ICON Global)
					ni = sec3Points
					nj = 1
				} else {
					return nil, fmt.Errorf("unsupported GRIB2 grid template %d (only regular lat/lon 0/40 and unstructured 100/101 supported)", gridTemplate)
				}
			}

		case 4: // Product Definition Section
			if len(secData) >= 9 {
				pdt := binary.BigEndian.Uint16(secData[7:9])
				if len(secData) >= 29 {
					paramCat = secData[9]
					paramNum = secData[10]
					timeUnit := secData[17] // 1 = hour
					forecastTime := binary.BigEndian.Uint32(secData[18:22])
					if timeUnit == 1 {
						stepHrs = int(forecastTime)
					} else if timeUnit == 0 { // minute
						stepHrs = int(forecastTime) / 60
					} else if timeUnit == 2 { // day
						stepHrs = int(forecastTime) * 24
					} else {
						stepHrs = int(forecastTime)
					}

					surfType = secData[22]
					scaleFactor := int8(secData[23])
					scaleVal := binary.BigEndian.Uint32(secData[24:28])
					surfVal = float64(scaleVal) * math.Pow10(-int(scaleFactor))
				}
				_ = pdt
			}

		case 5: // Data Representation Section
			if len(secData) >= 9 {
				sec5Points = int(binary.BigEndian.Uint32(secData[5:9]))
			}
			if len(secData) >= 11 {
				repTemplate = binary.BigEndian.Uint16(secData[9:11])
				if repTemplate == 0 { // Simple packing
					if len(secData) >= 21 {
						bits := binary.BigEndian.Uint32(secData[11:15])
						refVal = math.Float32frombits(bits)
						binScale = int16(binary.BigEndian.Uint16(secData[15:17]))
						decScale = int16(binary.BigEndian.Uint16(secData[17:19]))
						numBits = secData[19]
					}
				} else if repTemplate == 2 || repTemplate == 3 { // Complex packing & spatial differencing
					if len(secData) >= 47 {
						bits := binary.BigEndian.Uint32(secData[11:15])
						cpm.refVal = math.Float32frombits(bits)
						cpm.binScale = int16(binary.BigEndian.Uint16(secData[15:17]))
						cpm.decScale = int16(binary.BigEndian.Uint16(secData[17:19]))
						cpm.numBits = secData[19]
						cpm.originalType = secData[20]
						cpm.groupSplitting = secData[21]
						cpm.missingValMgmt = secData[22]
						cpm.primaryMissing = binary.BigEndian.Uint32(secData[23:27])
						cpm.secondaryMissing = binary.BigEndian.Uint32(secData[27:31])
						cpm.numGroups = int(binary.BigEndian.Uint32(secData[31:35]))
						cpm.refGroupWidths = secData[35]
						cpm.numBitsGroupWidths = secData[36]
						cpm.refGroupLengths = int(binary.BigEndian.Uint32(secData[37:41]))
						cpm.lenIncrementGroupLengths = int(secData[41])
						cpm.trueLenLastGroup = int(binary.BigEndian.Uint32(secData[42:46]))
						cpm.numBitsGroupLengths = secData[46]
						if repTemplate == 3 && len(secData) >= 49 {
							cpm.orderSpatialDiff = secData[47]
							cpm.numOctetsExtraDescriptor = secData[48]
						}
					}
				} else if repTemplate == 4 { // IEEE floating point
					if len(secData) >= 12 {
						numBits = secData[11] // 32 or 64
					}
				} else if repTemplate == 42 { // CCSDS compression (Template 5.42)
					if len(secData) >= 25 {
						bits := binary.BigEndian.Uint32(secData[11:15])
						refVal = math.Float32frombits(bits)
						binScale = readGRIBSignedInt16(secData[15:17])
						decScale = readGRIBSignedInt16(secData[17:19])
						numBits = secData[19]
						// secData[20] is originalType (octet 21)
						ccsdsOpt.flags = uint32(secData[21]) // octet 22
						ccsdsOpt.blockSize = int(secData[22]) // octet 23
						ccsdsOpt.rsi = int(binary.BigEndian.Uint16(secData[23:25])) // octet 24-25
					}
				}
			}

		case 6: // Bit-map Section
			if len(secData) >= 6 {
				indicator := secData[5]
				if indicator == 0 {
					hasBitmap = true
					bitmap = secData[6:]
				} else {
					hasBitmap = false
				}
			}

		case 7: // Data Section
			if len(secData) >= 5 {
				rawPayload := secData[5:]
				numPoints := ni * nj
				if numPoints <= 0 {
					if sec5Points > 0 {
						numPoints = sec5Points
					} else if sec3Points > 0 {
						numPoints = sec3Points
					}
				}

				var decoded []float32
				var err error

				if repTemplate == 0 { // Simple packing
					decoded, err = decodeSimplePacking(rawPayload, numPoints, refVal, binScale, decScale, numBits, hasBitmap, bitmap)
					if err != nil {
						return nil, fmt.Errorf("failed to decode simple packing: %w", err)
					}
				} else if repTemplate == 2 || repTemplate == 3 { // Complex packing
					decoded, err = decodeComplexPacking(rawPayload, numPoints, cpm, repTemplate, hasBitmap, bitmap)
					if err != nil {
						return nil, fmt.Errorf("failed to decode complex packing (template %d): %w", repTemplate, err)
					}
				} else if repTemplate == 4 { // IEEE Float32
					decoded, err = decodeIEEEFloats(rawPayload, numPoints)
					if err != nil {
						return nil, fmt.Errorf("failed to decode IEEE floats: %w", err)
					}
				} else if repTemplate == 42 { // CCSDS / AEC lossless compression
					decoded, err = decodeCCSDSPacking(rawPayload, numPoints, refVal, binScale, decScale, numBits, ccsdsOpt, hasBitmap, bitmap)
					if err != nil {
						return nil, fmt.Errorf("failed to decode CCSDS packing (template 42): %w", err)
					}
				} else {
					return nil, fmt.Errorf("unsupported GRIB2 data representation template %d (compression/packing format not supported)", repTemplate)
				}
				msg.Values = decoded
			}
		}

		offset += secLen
	}

	msg.StepHours = stepHrs
	msg.ValidTime = refTime.Add(time.Duration(stepHrs) * time.Hour)
	msg.ParamCategory = paramCat
	msg.ParamNumber = paramNum
	msg.SurfaceType = surfType
	msg.SurfaceValue = surfVal
	msg.Ni = ni
	msg.Nj = nj
	msg.La1 = la1
	msg.Lo1 = lo1
	msg.La2 = la2
	msg.Lo2 = lo2
	msg.Di = di
	msg.Dj = dj
	msg.ScanningMode = scanMode
	msg.DataPoints = len(msg.Values)

	return msg, nil
}

func readGRIBSignedInt16(b []byte) int16 {
	if len(b) < 2 {
		return 0
	}
	sign := (b[0] & 0x80) != 0
	val := int16(binary.BigEndian.Uint16(b) & 0x7FFF)
	if sign {
		return -val
	}
	return val
}

func readGRIBSigned(b []byte) int64 {
	if len(b) == 0 {
		return 0
	}
	sign := (b[0] & 0x80) != 0
	var mag uint64 = uint64(b[0] & 0x7F)
	for i := 1; i < len(b); i++ {
		mag = (mag << 8) | uint64(b[i])
	}
	if sign {
		return -int64(mag)
	}
	return int64(mag)
}

// decodeComplexPacking unpacks GRIB2 Complex Packing (DRT 5.2 / 5.3) with optional spatial differencing.
func decodeComplexPacking(payload []byte, numPoints int, cpm complexPackingMeta, repTemplate uint16, hasBitmap bool, bitmap []byte) ([]float32, error) {
	headerBytes := 0
	var x1, x2, minSD int64
	order := int(cpm.orderSpatialDiff)
	if repTemplate == 3 {
		extraOctets := int(cpm.numOctetsExtraDescriptor)
		if extraOctets <= 0 {
			extraOctets = 1
		}
		if order == 1 {
			if len(payload) < 2*extraOctets {
				return nil, fmt.Errorf("spatial differencing payload too short for order 1")
			}
			x1 = readGRIBSigned(payload[0:extraOctets])
			minSD = readGRIBSigned(payload[extraOctets : 2*extraOctets])
			headerBytes = 2 * extraOctets
		} else if order == 2 {
			if len(payload) < 3*extraOctets {
				return nil, fmt.Errorf("spatial differencing payload too short for order 2")
			}
			x1 = readGRIBSigned(payload[0:extraOctets])
			x2 = readGRIBSigned(payload[extraOctets : 2*extraOctets])
			minSD = readGRIBSigned(payload[2*extraOctets : 3*extraOctets])
			headerBytes = 3 * extraOctets
		}
	}

	br := newBitReader(payload[headerBytes:])

	// 1. Group references
	groupRefs := make([]uint32, cpm.numGroups)
	if cpm.numBits > 0 {
		for g := 0; g < cpm.numGroups; g++ {
			v, err := br.readBits(cpm.numBits)
			if err != nil {
				return nil, fmt.Errorf("failed reading groupRef %d: %w", g, err)
			}
			groupRefs[g] = v
		}
		br.alignToByte()
	}

	// 2. Group widths
	groupWidths := make([]uint8, cpm.numGroups)
	if cpm.numBitsGroupWidths > 0 {
		for g := 0; g < cpm.numGroups; g++ {
			v, err := br.readBits(cpm.numBitsGroupWidths)
			if err != nil {
				return nil, fmt.Errorf("failed reading groupWidth %d: %w", g, err)
			}
			groupWidths[g] = uint8(v) + cpm.refGroupWidths
		}
		br.alignToByte()
	} else {
		for g := 0; g < cpm.numGroups; g++ {
			groupWidths[g] = cpm.refGroupWidths
		}
	}

	// 3. Group lengths
	groupLengths := make([]int, cpm.numGroups)
	if cpm.numBitsGroupLengths > 0 {
		for g := 0; g < cpm.numGroups; g++ {
			v, err := br.readBits(cpm.numBitsGroupLengths)
			if err != nil {
				return nil, fmt.Errorf("failed reading groupLength %d: %w", g, err)
			}
			groupLengths[g] = int(v)*cpm.lenIncrementGroupLengths + cpm.refGroupLengths
		}
		br.alignToByte()
	} else {
		for g := 0; g < cpm.numGroups; g++ {
			groupLengths[g] = cpm.refGroupLengths
		}
	}
	if cpm.numGroups > 0 && cpm.trueLenLastGroup > 0 {
		groupLengths[cpm.numGroups-1] = cpm.trueLenLastGroup
	}

	// 4. Data unpacking
	Y := make([]int64, numPoints)
	ptIdx := 0
	for g := 0; g < cpm.numGroups && ptIdx < numPoints; g++ {
		w := groupWidths[g]
		l := groupLengths[g]
		ref := int64(groupRefs[g])

		if w == 0 {
			for k := 0; k < l && ptIdx < numPoints; k++ {
				Y[ptIdx] = ref
				ptIdx++
			}
		} else {
			for k := 0; k < l && ptIdx < numPoints; k++ {
				v, err := br.readBits(w)
				if err != nil {
					break
				}
				Y[ptIdx] = ref + int64(v)
				ptIdx++
			}
		}
	}

	// 5. Undo spatial differencing
	if repTemplate == 3 {
		if order == 1 {
			Y[0] = x1
			for i := 1; i < numPoints; i++ {
				Y[i] = Y[i] + minSD + Y[i-1]
			}
		} else if order == 2 {
			Y[0] = x1
			if numPoints > 1 {
				Y[1] = x2
			}
			for i := 2; i < numPoints; i++ {
				Y[i] = Y[i] + minSD + 2*Y[i-1] - Y[i-2]
			}
		}
	}

	// 6. Scale to float32
	bScaleMult := math.Pow(2.0, float64(cpm.binScale))
	dScaleDiv := math.Pow10(int(cpm.decScale))
	out := make([]float32, numPoints)
	for i := 0; i < numPoints; i++ {
		if hasBitmap && !isBitSet(bitmap, i) {
			out[i] = float32(math.NaN())
		} else {
			out[i] = float32((float64(cpm.refVal) + float64(Y[i])*bScaleMult) / dScaleDiv)
		}
	}

	return out, nil
}

// decodeSimplePacking unpacks standard GRIB2 Simple Packing (Template 5.0 / 7.0).
// Formula: Y = (R + X * 2^E) / 10^D
func decodeSimplePacking(payload []byte, numPoints int, refVal float32, binScale, decScale int16, numBits uint8, hasBitmap bool, bitmap []byte) ([]float32, error) {
	if numBits == 0 {
		// Constant field (all values equal refVal)
		constantVal := float32(float64(refVal) / math.Pow10(int(decScale)))
		res := make([]float32, numPoints)
		for i := range res {
			res[i] = constantVal
		}
		return res, nil
	}

	bScaleMult := math.Pow(2.0, float64(binScale))
	dScaleDiv := math.Pow10(int(decScale))

	out := make([]float32, numPoints)
	bitReader := newBitReader(payload)

	for i := 0; i < numPoints; i++ {
		if hasBitmap && !isBitSet(bitmap, i) {
			out[i] = float32(math.NaN())
			continue
		}

		rawInt, err := bitReader.readBits(numBits)
		if err != nil {
			if i > 0 {
				break
			}
			return nil, err
		}

		val := (float64(refVal) + float64(rawInt)*bScaleMult) / dScaleDiv
		out[i] = float32(val)
	}

	return out, nil
}

// decodeIEEEFloats unpacks raw 32-bit floating point numbers.
func decodeIEEEFloats(payload []byte, numPoints int) ([]float32, error) {
	if len(payload) < numPoints*4 {
		numPoints = len(payload) / 4
	}
	out := make([]float32, numPoints)
	for i := 0; i < numPoints; i++ {
		bits := binary.BigEndian.Uint32(payload[i*4 : (i+1)*4])
		out[i] = math.Float32frombits(bits)
	}
	return out, nil
}

func isBitSet(bitmap []byte, idx int) bool {
	byteIdx := idx / 8
	if byteIdx >= len(bitmap) {
		return false
	}
	bitIdx := 7 - (idx % 8)
	return (bitmap[byteIdx] & (1 << bitIdx)) != 0
}

// bitReader allows reading arbitrary bit counts (1 to 32 bits) from a byte slice.
type bitReader struct {
	data   []byte
	bitPos int
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data, bitPos: 0}
}

func (r *bitReader) readBits(n uint8) (uint32, error) {
	if n == 0 {
		return 0, nil
	}
	if n > 32 {
		return 0, fmt.Errorf("cannot read more than 32 bits at once")
	}

	startByte := r.bitPos / 8
	startBit := r.bitPos % 8
	totalBits := int(n)

	if startByte+((startBit+totalBits+7)/8) > len(r.data)+1 {
		return 0, io.EOF
	}

	var result uint32
	bitsLeft := totalBits

	for bitsLeft > 0 {
		byteIdx := r.bitPos / 8
		if byteIdx >= len(r.data) {
			return 0, io.EOF
		}
		bitInByte := r.bitPos % 8
		availableInByte := 8 - bitInByte

		take := bitsLeft
		if take > availableInByte {
			take = availableInByte
		}

		mask := uint32((1 << take) - 1)
		shift := uint32(availableInByte - take)
		part := (uint32(r.data[byteIdx]) >> shift) & mask

		result = (result << take) | part
		r.bitPos += take
		bitsLeft -= take
	}

	return result, nil
}

func (r *bitReader) alignToByte() {
	rem := r.bitPos % 8
	if rem != 0 {
		r.bitPos += (8 - rem)
	}
}

// decodeCCSDSPacking decodes GRIB2 Data Representation Template 5.42 (CCSDS / AEC lossless compression).
func decodeCCSDSPacking(payload []byte, numPoints int, refVal float32, binScale, decScale int16, numBits uint8, opt ccsdsMeta, hasBitmap bool, bitmap []byte) ([]float32, error) {
	if numPoints <= 0 {
		return nil, nil
	}

	if numBits == 0 {
		out := make([]float32, numPoints)
		val := float32(refVal)
		for i := range out {
			out[i] = val
		}
		return out, nil
	}

	rawCount := numPoints
	if hasBitmap && len(bitmap) > 0 {
		count := 0
		for i := 0; i < numPoints; i++ {
			byteIdx := i / 8
			bitIdx := 7 - (i % 8)
			if byteIdx < len(bitmap) && ((bitmap[byteIdx]>>bitIdx)&1) == 1 {
				count++
			}
		}
		rawCount = count
	}

	blockSize := opt.blockSize
	if blockSize <= 0 {
		blockSize = 16
	}
	rsi := opt.rsi
	if rsi <= 0 {
		rsi = 128
	}

	params := aec.Params{
		BitsPerSample: int(numBits),
		BlockSize:     blockSize,
		RSI:           rsi,
		Flags:         int(opt.flags),
		NumValues:     rawCount,
	}

	decodedInts, err := aec.Decode(payload, params)
	if err != nil {
		return nil, fmt.Errorf("aec decode error: %w", err)
	}

	scaleBin := math.Pow(2.0, float64(binScale))
	scaleDec := math.Pow(10.0, -float64(decScale))

	out := make([]float32, numPoints)
	valIdx := 0

	for i := 0; i < numPoints; i++ {
		if hasBitmap && len(bitmap) > 0 {
			byteIdx := i / 8
			bitIdx := 7 - (i % 8)
			if byteIdx >= len(bitmap) || ((bitmap[byteIdx]>>bitIdx)&1) == 0 {
				out[i] = float32(math.NaN())
				continue
			}
		}

		if valIdx < len(decodedInts) {
			rawVal := decodedInts[valIdx]
			valIdx++
			scaled := (float64(refVal) + float64(rawVal)*scaleBin) * scaleDec
			out[i] = float32(scaled)
		} else {
			out[i] = float32(math.NaN())
		}
	}

	return out, nil
}

// ToRawGridSlice converts a decoded GRIB2 message to a canonical RawGridSlice.
func (m *Message) ToRawGridSlice(canonicalVar string) *model.RawGridSlice {
	return &model.RawGridSlice{
		Variable:  canonicalVar,
		ValidTime: m.ValidTime,
		StepHours: m.StepHours,
		NLats:     m.Nj,
		NLons:     m.Ni,
		LatStart:  m.La1,
		LatEnd:    m.La2,
		LatStep:   m.Dj,
		LonStart:  m.Lo1,
		LonEnd:    m.Lo2,
		LonStep:   m.Di,
		Data:      m.Values,
	}
}
