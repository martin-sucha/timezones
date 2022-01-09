package timezones

import (
	"reflect"
	"testing"
	"time"
)

func TestLocationTemplate_NewLocation_UTC(t *testing.T) {
	loc, err := LocationTemplate{
		Name: "MyUTC",
		Zones: []Zone{
			{
				Name:   "MyUTC",
				Offset: 0,
				IsDST:  false,
			},
		},
		Changes: nil,
		Extend:  "",
	}.NewLocation()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ti := time.Date(2022, time.January, 9, 8, 10, 15, 0, loc)
	got := ti.Format("2006-01-02 15:04:05 -0700 MST")
	expected := "2022-01-09 08:10:15 +0000 MyUTC"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if ti.IsDST() {
		t.Fatal("true isDST is unexpected")
	}
}

func TestLocationTemplate_NewLocation_FixedOffset(t *testing.T) {
	loc, err := LocationTemplate{
		Name: "MyFixed",
		Zones: []Zone{
			{
				Name:   "MyFixed",
				Offset: 2*time.Hour + 23*time.Minute,
				IsDST:  false,
			},
		},
		Changes: nil,
		Extend:  "",
	}.NewLocation()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ti := time.Date(2022, time.January, 9, 8, 10, 15, 0, loc)
	got := ti.Format("2006-01-02 15:04:05 -0700 MST")
	expected := "2022-01-09 08:10:15 +0223 MyFixed"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if ti.IsDST() {
		t.Fatal("expecting isDST to be false")
	}
}

func TestLocationTemplate_NewLocation_Changes(t *testing.T) {
	loc, err := LocationTemplate{
		Name: "MyChanges",
		Zones: []Zone{
			{
				Name:   "Std",
				Offset: 2*time.Hour + 23*time.Minute,
				IsDST:  false,
			},
			{
				Name:   "Dst",
				Offset: 2*time.Hour + 53*time.Minute,
				IsDST:  true,
			},
		},
		Changes: []Change{
			{
				Start:     time.Date(2022, time.January, 9, 10, 0, 0, 0, time.UTC),
				ZoneIndex: 1,
			},
			{
				Start:     time.Date(2022, time.January, 9, 11, 0, 0, 0, time.UTC),
				ZoneIndex: 0,
			},
		},
		Extend: "",
	}.NewLocation()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ti := time.Date(2022, time.January, 9, 12, 22, 59, 0, loc)
	got := ti.Format("2006-01-02 15:04:05 -0700 MST")
	expected := "2022-01-09 12:22:59 +0223 Std"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if ti.IsDST() {
		t.Fatal("expecting isDST to be false")
	}

	ti = ti.Add(1 * time.Second) // time moves 30 minutes forward
	got = ti.Format("2006-01-02 15:04:05 -0700 MST")
	expected = "2022-01-09 12:53:00 +0253 Dst"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if !ti.IsDST() {
		t.Fatal("expecting isDST to be true")
	}
}

func TestLocationTemplate_NewLocation_ExtendOnly(t *testing.T) {
	loc, err := LocationTemplate{
		Name:    "MyExt",
		Zones:   nil,
		Changes: nil,
		Extend:  "<MyExt>-02:23:00<MyExtDST>-03:23:00,M1.2.3/10:00:00,M2.3.4/10:00:00",
	}.NewLocation()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ti := time.Date(2022, time.January, 9, 8, 10, 15, 0, loc)
	got := ti.Format("2006-01-02 15:04:05 -0700 MST")
	expected := "2022-01-09 08:10:15 +0223 MyExt"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if ti.IsDST() {
		t.Fatal("expecting isDST to be false")
	}

	ti = time.Date(2022, time.January, 12, 9, 59, 59, 0, loc)
	got = ti.Format("2006-01-02 15:04:05 -0700 MST")
	expected = "2022-01-12 09:59:59 +0223 MyExt"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if ti.IsDST() {
		t.Fatal("expecting isDST to be false")
	}

	ti = ti.Add(1 * time.Second) // At 10:00, local clock moves to 11:00
	got = ti.Format("2006-01-02 15:04:05 -0700 MST")
	expected = "2022-01-12 11:00:00 +0323 MyExtDST"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
	if !ti.IsDST() {
		t.Fatal("expecting isDST to be true")
	}
}

func benchTemplate() LocationTemplate {
	changes := make([]Change, 100)
	for i := 0; i < len(changes); i += 2 {
		changes[i].Start = time.Date(1980+i, time.January, 9, 10, 0, 0, 0, time.UTC)
		changes[i].ZoneIndex = 1
		changes[i+1].Start = time.Date(1980+i, time.January, 9, 11, 0, 0, 0, time.UTC)
		changes[i+1].ZoneIndex = 0
	}
	return LocationTemplate{
		Name: "MyChanges",
		Zones: []Zone{
			{
				Name:   "Std",
				Offset: 2*time.Hour + 23*time.Minute,
				IsDST:  false,
			},
			{
				Name:   "Dst",
				Offset: 2*time.Hour + 53*time.Minute,
				IsDST:  true,
			},
		},
		Changes: changes,
		Extend:  "",
	}
}

var benchTmpl LocationTemplate

func BenchmarkAllocTemplate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchTmpl = benchTemplate()
	}
}

var benchLoc *time.Location

func BenchmarkLocationTemplate_NewLocation(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		template := benchTemplate()
		loc, err := template.NewLocation()
		if err != nil {
			b.Fatal(err)
		}
		benchLoc = loc
	}
}

var benchTZData []byte

func BenchmarkLocationTemplate_tzdata(b *testing.B) {
	template := benchTemplate()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, err := buildTZData(&template)
		if err != nil {
			b.Fatal(err)
		}
		benchTZData = buf
	}
}

var benchLoadLocation *time.Location

func BenchmarkLoadLocation(b *testing.B) {
	template := benchTemplate()
	buf, err := buildTZData(&template)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loc, err := time.LoadLocationFromTZData("Test", buf)
		if err != nil {
			b.Fatal(err)
		}
		benchLoadLocation = loc
	}
}

func TestZoneDesignations_Add(t *testing.T) {
	var zd zoneDesignations
	expect := func(names []string, offsets []int) {
		if !reflect.DeepEqual(names, zd.names) {
			t.Fatalf("expected names %+v, got %+v", names, zd.names)
		}
		if !reflect.DeepEqual(offsets, zd.offsets) {
			t.Fatalf("expected offsets %+v, got %+v", offsets, zd.offsets)
		}
	}
	zd.add("WEST")
	expect([]string{"WEST"}, []int{0})
	zd.add("REST")
	expect([]string{"WEST", "REST"}, []int{0, 5})
	zd.add("EST")
	expect([]string{"WEST", "REST"}, []int{0, 5, 1})
	zd.add("REST")
	expect([]string{"WEST", "REST"}, []int{0, 5, 1, 5})
}

func TestFill(t *testing.T) {
	fill(nil, 1)
	buf := make([]byte, 113)
	fill(buf, 42)
	for i := range buf {
		if buf[i] != 42 {
			t.Fatalf("unexpected value %d in buffer at index %d", buf[i], i)
		}
	}
}
