package grib2

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func TestBitReader(t *testing.T) {
	// 2 bytes: 0b10101100, 0b11110000 -> 0xAC, 0xF0
	data := []byte{0xAC, 0xF0}
	br := newBitReader(data)

	// Read 4 bits: 1010 (10)
	v1, err := br.readBits(4)
	if err != nil || v1 != 10 {
		t.Fatalf("expected 10, got %d, err: %v", v1, err)
	}

	// Read 6 bits: 110011 (51)
	v2, err := br.readBits(6)
	if err != nil || v2 != 51 {
		t.Fatalf("expected 51, got %d, err: %v", v2, err)
	}

	// Read 6 bits: 110000 (48)
	v3, err := br.readBits(6)
	if err != nil || v3 != 48 {
		t.Fatalf("expected 48, got %d, err: %v", v3, err)
	}
}

func TestSimplePackingDecoder(t *testing.T) {
	// Test unpacking with reference value 10.0, binScale 0, decScale 1 (divide by 10), numBits 8
	// Raw integer values: 0, 10, 20
	// Decoded: (10.0 + 0)/10 = 1.0; (10.0 + 10)/10 = 2.0; (10.0 + 20)/10 = 3.0
	payload := []byte{0, 10, 20}
	numPoints := 3
	refVal := float32(10.0)
	binScale := int16(0)
	decScale := int16(1)
	numBits := uint8(8)

	decoded, err := decodeSimplePacking(payload, numPoints, refVal, binScale, decScale, numBits, false, nil)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	expected := []float32{1.0, 2.0, 3.0}
	for i, v := range decoded {
		if math.Abs(float64(v-expected[i])) > 1e-4 {
			t.Errorf("at index %d: expected %f, got %f", i, expected[i], v)
		}
	}
}

func TestGRIB2ParseSynthetic(t *testing.T) {
	buf := new(bytes.Buffer)

	// Section 0: Indicator (16 bytes)
	buf.WriteString("GRIB")
	buf.Write([]byte{0, 0, 0, 2}) // discipline 0, edition 2
	totalLenPlaceholder := buf.Len()
	binary.Write(buf, binary.BigEndian, uint64(0)) // Will update at end

	// Section 1: Identification (21 bytes)
	s1 := new(bytes.Buffer)
	binary.Write(s1, binary.BigEndian, uint32(21))
	s1.WriteByte(1) // Sec 1
	binary.Write(s1, binary.BigEndian, uint16(7)) // Center: NCEP
	binary.Write(s1, binary.BigEndian, uint16(0)) // Subcenter
	s1.WriteByte(2) // Master table
	s1.WriteByte(1) // Local table
	s1.WriteByte(1) // Ref time sig
	binary.Write(s1, binary.BigEndian, uint16(2026)) // Year
	s1.Write([]byte{8, 30, 6, 0, 0}) // Month 8, Day 30, Hour 6, Min 0, Sec 0
	s1.Write([]byte{0, 1}) // Prod status, type
	buf.Write(s1.Bytes())

	// Section 3: Grid Definition (72 bytes for Template 3.0)
	s3 := new(bytes.Buffer)
	binary.Write(s3, binary.BigEndian, uint32(72))
	s3.WriteByte(3) // Sec 3
	s3.WriteByte(0) // Source
	binary.Write(s3, binary.BigEndian, uint32(4)) // 4 points (2x2)
	s3.Write([]byte{0, 0}) // optional octets
	binary.Write(s3, binary.BigEndian, uint16(0)) // Grid Template 0 (Lat/Lon)
	s3.WriteByte(6) // Earth shape
	s3.Write(make([]byte, 15)) // radius scale/val
	binary.Write(s3, binary.BigEndian, uint32(2)) // Ni = 2
	binary.Write(s3, binary.BigEndian, uint32(2)) // Nj = 2
	binary.Write(s3, binary.BigEndian, uint32(0)) // basic angle
	binary.Write(s3, binary.BigEndian, uint32(0)) // subdivisions
	binary.Write(s3, binary.BigEndian, int32(90000000)) // La1 = 90.0
	binary.Write(s3, binary.BigEndian, int32(0)) // Lo1 = 0.0
	s3.WriteByte(48) // flags
	binary.Write(s3, binary.BigEndian, int32(89750000)) // La2 = 89.75
	binary.Write(s3, binary.BigEndian, int32(250000)) // Lo2 = 0.25
	binary.Write(s3, binary.BigEndian, uint32(250000)) // Di = 0.25
	binary.Write(s3, binary.BigEndian, uint32(250000)) // Dj = 0.25
	s3.WriteByte(0) // scan mode
	buf.Write(s3.Bytes())

	// Section 4: Product Definition (34 bytes for Template 4.0)
	s4 := new(bytes.Buffer)
	binary.Write(s4, binary.BigEndian, uint32(34))
	s4.WriteByte(4)                                // Sec 4
	binary.Write(s4, binary.BigEndian, uint16(0))  // coord values
	binary.Write(s4, binary.BigEndian, uint16(0))  // PDT 4.0
	s4.WriteByte(2)                                // Param category 2 (Momentum)
	s4.WriteByte(2)                                // Param number 2 (UGRD)
	s4.Write(make([]byte, 6))                      // gen process
	s4.WriteByte(1)                                // Time unit = hour
	binary.Write(s4, binary.BigEndian, uint32(12)) // Step = 12h
	s4.WriteByte(103)                              // 10 m above ground
	s4.WriteByte(0)
	binary.Write(s4, binary.BigEndian, uint32(10))
	s4.Write(make([]byte, 6)) // padding to 34 bytes
	if s4.Len() != 34 {
		t.Fatalf("s4 len expected 34, got %d", s4.Len())
	}
	buf.Write(s4.Bytes())

	// Section 5: Data Representation (21 bytes for Template 5.0)
	s5 := new(bytes.Buffer)
	binary.Write(s5, binary.BigEndian, uint32(21))
	s5.WriteByte(5) // Sec 5
	binary.Write(s5, binary.BigEndian, uint32(4)) // 4 data points
	binary.Write(s5, binary.BigEndian, uint16(0)) // Template 5.0 (Simple packing)
	binary.Write(s5, binary.BigEndian, math.Float32bits(5.0)) // RefVal = 5.0
	binary.Write(s5, binary.BigEndian, int16(0)) // BinScale = 0
	binary.Write(s5, binary.BigEndian, int16(0)) // DecScale = 0
	s5.WriteByte(8) // 8 bits per value
	s5.WriteByte(0) // float type
	buf.Write(s5.Bytes())

	// Section 6: Bit-map (6 bytes)
	s6 := new(bytes.Buffer)
	binary.Write(s6, binary.BigEndian, uint32(6))
	s6.WriteByte(6) // Sec 6
	s6.WriteByte(255) // No bitmap
	buf.Write(s6.Bytes())

	// Section 7: Data Section (5 + 4 bytes = 9 bytes)
	s7 := new(bytes.Buffer)
	binary.Write(s7, binary.BigEndian, uint32(9))
	s7.WriteByte(7) // Sec 7
	s7.Write([]byte{0, 1, 2, 3}) // 4 values: 5+0=5, 5+1=6, 5+2=7, 5+3=8
	buf.Write(s7.Bytes())

	// Section 8: End Section (4 bytes)
	buf.WriteString("7777")

	// Update total length in Section 0
	allBytes := buf.Bytes()
	binary.BigEndian.PutUint64(allBytes[totalLenPlaceholder:totalLenPlaceholder+8], uint64(len(allBytes)))

	// Parse
	msg, err := Parse(allBytes)
	if err != nil {
		t.Fatalf("Parse synthetic GRIB2 failed: %v", err)
	}

	if msg.StepHours != 12 {
		t.Errorf("expected StepHours 12, got %d", msg.StepHours)
	}
	if msg.Ni != 2 || msg.Nj != 2 {
		t.Errorf("expected 2x2 grid, got %dx%d", msg.Ni, msg.Nj)
	}
	if msg.DataPoints != 4 {
		t.Fatalf("expected 4 points, got %d", msg.DataPoints)
	}
	if len(msg.Values) != 4 {
		t.Fatalf("expected 4 values, got %d", len(msg.Values))
	}
	expectedVals := []float32{5.0, 6.0, 7.0, 8.0}
	for i, v := range msg.Values {
		if math.Abs(float64(v-expectedVals[i])) > 1e-4 {
			t.Errorf("val %d: expected %f, got %f", i, expectedVals[i], v)
		}
	}
}

func TestParseRealSample(t *testing.T) {
	data, err := os.ReadFile("/tmp/sample_gust.grib2")
	if err != nil {
		t.Skip("no /tmp/sample_gust.grib2 found")
	}

	msg, err := Parse(data)
	if err != nil {
		t.Fatalf("failed to parse real sample: %v", err)
	}

	t.Logf("Parsed real message: Ni=%d, Nj=%d, DataPoints=%d, Step=%d, RefTime=%s",
		msg.Ni, msg.Nj, msg.DataPoints, msg.StepHours, msg.ReferenceTime)
	var minVal, maxVal float32 = 1e9, -1e9
	var nonZeroCount int
	for _, v := range msg.Values {
		if v != 0 {
			nonZeroCount++
		}
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	t.Logf("Stats: NonZero=%d / %d, Min=%.3f, Max=%.3f", nonZeroCount, len(msg.Values), minVal, maxVal)
}

func TestInspectECMWFLocal(t *testing.T) {
	dataU, err := os.ReadFile("/Users/jaclar/.gemini/antigravity/brain/7ca50455-2971-4c92-b814-a64a679fc45b/scratch/ecmwf_10u.grib2")
	if err != nil {
		t.Skipf("No ecmwf_10u.grib2 found: %v", err)
	}
	dataV, err := os.ReadFile("/Users/jaclar/.gemini/antigravity/brain/7ca50455-2971-4c92-b814-a64a679fc45b/scratch/ecmwf_10v.grib2")
	if err != nil {
		t.Skipf("No ecmwf_10v.grib2 found: %v", err)
	}

	msgU, err := Parse(dataU)
	if err != nil {
		t.Fatalf("Failed to parse 10u: %v", err)
	}
	msgV, err := Parse(dataV)
	if err != nil {
		t.Fatalf("Failed to parse 10v: %v", err)
	}

	t.Logf("10u Grid: Ni=%d, Nj=%d, La1=%f, La2=%f, Lo1=%f, Lo2=%f, Di=%f, Dj=%f, ScanMode=%d",
		msgU.Ni, msgU.Nj, msgU.La1, msgU.La2, msgU.Lo1, msgU.Lo2, msgU.Di, msgU.Dj, msgU.ScanningMode)

	var minU, maxU, minV, maxV float32 = 1e9, -1e9, 1e9, -1e9
	for _, u := range msgU.Values {
		if u < minU {
			minU = u
		}
		if u > maxU {
			maxU = u
		}
	}
	for _, v := range msgV.Values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	t.Logf("ECMWF 10u min=%.3f max=%.3f | 10v min=%.3f max=%.3f", minU, maxU, minV, maxV)

	// Check North Pole (row 0), Equator (row 360), South Pole (row 720)
	t.Logf("Row 0 (Lat 90.0): U[0]=%.3f, V[0]=%.3f", msgU.Values[0], msgV.Values[0])
	t.Logf("Row 360 (Lat 0.0, Lon 0.0): U[360*1440]=%.3f, V[360*1440]=%.3f", msgU.Values[360*1440], msgV.Values[360*1440])
	t.Logf("Row 720 (Lat -90.0): U[720*1440]=%.3f, V[720*1440]=%.3f", msgU.Values[720*1440], msgV.Values[720*1440])

	// Check Grenada (lat 12.0, lon -61.75 -> lon 298.25)
	// If Lo1 = 180.0:
	// Longitude 298.25 is (298.25 - 180.0) / 0.25 = 473 columns into the raw row!
	idxCorrect := 312*1440 + 473
	uCorr := float64(msgU.Values[idxCorrect])
	vCorr := float64(msgV.Values[idxCorrect])
	spdCorr := math.Hypot(uCorr, vCorr) * 1.943844
	dirCorr := math.Mod(180.0+math.Atan2(uCorr, vCorr)*180.0/math.Pi, 360.0)
	t.Logf("ECMWF at REAL Grenada (lat 12.0, lon -61.75 / 298.25°E): U=%.2f m/s, V=%.2f m/s -> Spd=%.1f kts, Dir=%.1f°",
		uCorr, vCorr, spdCorr, dirCorr)

	if spdCorr < 8.0 || spdCorr > 15.0 {
		t.Errorf("expected trade wind speed ~10-12 knots, got %.1f", spdCorr)
	}
	if dirCorr < 60.0 || dirCorr > 110.0 {
		t.Errorf("expected ENE trade wind direction ~70-90°, got %.1f", dirCorr)
	}
}
