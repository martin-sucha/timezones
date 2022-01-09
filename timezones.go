// Package timezones builds new time.Locations.
package timezones

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

// Zone describes a single zone.
type Zone struct {
	// Name of the zone.
	Name string

	// Offset to add to UTC time to get local time.
	// Zones with positive offsets are east of UTC.
	Offset time.Duration

	// IsDST reports whether Daylight Savings Time is in effect.
	IsDST bool
}

// Change describes a change from one Zone to another.
type Change struct {
	// Start is time when the previous zone changes to ZoneIndex.
	Start time.Time

	// ZoneIndex which takes effect since Start.
	// Must be in range 0 to len(Zones)-1.
	ZoneIndex int
}

// Template describes how to build a time.Location.
type Template struct {
	// Name of the new location.
	Name string

	// Zones lists local zones.
	// At the beginning of time, Zone at index 0 applies.
	// When that zone changes to another zone is specified in Changes.
	// Maximum of 254 zones can be present.
	Zones []Zone

	// Changes specifies zone transitions.
	// Changes Start times must be in strictly increasing order.
	// If Extend is non-empty, the ZoneIndex of the last Change is ignored, Extend is used instead.
	// Changes might be empty, in that case Extend must be non-empty.
	Changes []Change

	// If Extend is non-empty, it replaces the definition of zones since the last change.
	// If there is at most one zone specified by Zones and Changes, Extend applies since the beginning of time.
	// Extend is a TZ string conforming to RFC 8536, section 3.3.
	Extend string
}

// NewLocation creates a new time.Location from the template.
func NewLocation(template Template) (*time.Location, error) {
	tzData, err := buildTZData(&template)
	if err != nil {
		return nil, err
	}
	return time.LoadLocationFromTZData(template.Name, tzData)
}

// TZData converts the template to TZif data.
func TZData(template Template) ([]byte, error) {
	return buildTZData(&template)
}

const headerSize = 4 + 1 + 15 + 6*4 // magic + ver + unused + 6x count

// maxUserZones is how many zones a user can specify.
// There are max 255 possible local time type records in a TZif file.
// We reserve zone 0 for the first zone, which must be unused by transitions
// because Go's time package does not follow the RFC 8536 exactly and chooses nonzero record in
// some cases if zone 0 is used in transitions, see time.Location.lookupFirstZone.
const maxUserZones = 254

// buildTZData builds TZIF description from location template.
// See https://datatracker.ietf.org/doc/html/rfc8536
//
// If V2+ data is present in TZIF stream, readers should use V2 data.
// Go ignores the V1 data completely, in that case, so buildTZData uses empty V1 data block.
func buildTZData(template *Template) ([]byte, error) {
	if len(template.Zones) > maxUserZones {
		return nil, fmt.Errorf("too many zones (%d), max is %d", len(template.Zones), maxUserZones)
	}
	if len(template.Zones) == 0 && template.Extend == "" {
		return nil, fmt.Errorf("either zones or extend string need to be present")
	}
	if len(template.Changes) > math.MaxUint32 {
		return nil, fmt.Errorf("too many changes (%d), max is %d", len(template.Changes), math.MaxUint32)
	}

	size := headerSize + // v1 header + empty v1 data block
		headerSize // v2 header
	// We only write transition times, transition types, local time type records, time zone designations.
	// Go seems to ignore standard/wall indicators and UT/local indicators, which seems like a bug in Go, so
	// we include them.
	// Go does not read leap seconds, so we don't include any.
	timecnt := len(template.Changes)
	isutcnt := timecnt
	isstdcnt := timecnt
	typecnt := len(template.Zones) + 1 // first zone is special
	var firstZone Zone
	if len(template.Zones) > 0 {
		firstZone = template.Zones[0]
	}
	zd := zoneDesignations{
		names:   make([]string, 0, typecnt),
		offsets: make([]int, 0, typecnt),
	}
	// Build time zone designations.
	// We need to deduplicate them because the index into time zone designations is only a single byte.
	zd.add(firstZone.Name)
	for i := range template.Zones {
		zd.add(template.Zones[i].Name)
	}
	if zd.charcnt > math.MaxUint8 {
		return nil, fmt.Errorf("time zone designators don't fit into limit, charcnt=%d", zd.charcnt)
	}
	// Add the size of the V2 data block.
	dataBlockSize := timecnt*8 + timecnt + typecnt*6 + zd.charcnt + isstdcnt + isutcnt
	size += dataBlockSize
	// Add the size of footer.
	size += 2 + len(template.Extend)

	data := make([]byte, size)
	// V1 header
	v1Header, rest := data[:headerSize], data[headerSize:]
	v1Header[0] = 'T'
	v1Header[1] = 'Z'
	v1Header[2] = 'i'
	v1Header[3] = 'f'
	v1Header[4] = '3' // version
	// V2 header
	v2Header, rest := rest[:headerSize], rest[headerSize:]
	v2Header[0] = 'T'
	v2Header[1] = 'Z'
	v2Header[2] = 'i'
	v2Header[3] = 'f'
	v2Header[4] = '3' // version
	binary.BigEndian.PutUint32(v2Header[20:24], uint32(isutcnt))
	binary.BigEndian.PutUint32(v2Header[24:28], uint32(isstdcnt))
	binary.BigEndian.PutUint32(v2Header[32:36], uint32(timecnt))
	binary.BigEndian.PutUint32(v2Header[36:40], uint32(typecnt))
	binary.BigEndian.PutUint32(v2Header[40:44], uint32(zd.charcnt))
	// V2 data block
	// transition times
	transitionTimes, rest := rest[:timecnt*8], rest[timecnt*8:]
	for i := range template.Changes {
		if i > 0 && !template.Changes[i].Start.After(template.Changes[i-1].Start) {
			return nil, fmt.Errorf("zone changes must be in strictly ascending order")
		}
		binary.BigEndian.PutUint64(transitionTimes[:8], uint64(template.Changes[i].Start.Unix()))
		transitionTimes = transitionTimes[8:]
	}
	// transition types
	transitionTypes, rest := rest[:timecnt], rest[timecnt:]
	for i := range template.Changes {
		// We add 1 to ZoneIndex because local time type record 0 is used by firstZone.
		transitionTypes[0] = byte(template.Changes[i].ZoneIndex + 1)
		transitionTypes = transitionTypes[1:]
	}
	// local time type records
	localTimeType, rest := rest[:typecnt*6], rest[typecnt*6:]
	localTimeType = putLocalTimeTypeRecord(localTimeType, firstZone.Offset, firstZone.IsDST, zd.offsets[0])
	for i := range template.Zones {
		localTimeType = putLocalTimeTypeRecord(localTimeType, template.Zones[i].Offset, template.Zones[i].IsDST, zd.offsets[i+1])
	}
	// time zone designations
	for i := range zd.names {
		n := copy(rest, zd.names[i])
		rest = rest[n+1:]
	}
	// no leap second records
	// standard/wall indicators and UT/local indicators
	// We are always using UT, so all indicators are 1.
	fill(rest[:isstdcnt+isutcnt], 1)
	rest = rest[isstdcnt+isutcnt:]
	// footer
	rest[0], rest = '\n', rest[1:]
	copy(rest, template.Extend)
	rest = rest[len(template.Extend):]
	rest[0], rest = '\n', rest[1:]

	// everything written, do a sanity check
	if len(rest) != 0 {
		panic("some data was not written")
	}

	return data, nil
}

// zoneDesignations builds the buffer that holds zone names.
type zoneDesignations struct {
	charcnt int
	names   []string
	offsets []int
}

func (zd *zoneDesignations) add(name string) {
	for i := 0; i < len(zd.names); i++ {
		if strings.HasSuffix(zd.names[i], name) {
			// Reuse existing record.
			zd.offsets = append(zd.offsets, zd.offsets[i]+len(zd.names[i])-len(name))
			return
		}
	}
	// Add new record.
	zd.names = append(zd.names, name)
	zd.offsets = append(zd.offsets, zd.charcnt)
	zd.charcnt += len(name) + 1
}

func putLocalTimeTypeRecord(buf []byte, offset time.Duration, isDST bool, nameOffset int) []byte {
	record, rest := buf[:6], buf[6:]
	binary.BigEndian.PutUint32(record[0:4], uint32(offset/time.Second))
	if isDST {
		record[4] = 1
	}
	record[5] = byte(nameOffset)
	return rest
}

// fill the buffer with a constant value.
func fill(buffer []byte, value byte) {
	l := len(buffer)
	if l == 0 {
		return
	}
	buffer[0] = value
	for i := 1; i < l; i *= 2 {
		copy(buffer[i:], buffer[:i])
	}
}
