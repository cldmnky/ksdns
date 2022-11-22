package file

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/cldmnky/ksdns/pkg/zupd/changelog"
	"github.com/cldmnky/ksdns/pkg/zupd/config"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFile(t *testing.T) {
	for scenario, fn := range map[string]func(
		*testing.T, *File){
		"NewFile":   testNewFile,
		"Insert":    testInsert,
		"ChangeLog": testChangeLog,
	} {
		t.Run(scenario, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "store-test")
			require.NoError(t, err)
			defer os.RemoveAll(dir)
			config, err := config.NewConfig(
				"127.0.0.1",
				"53",
				"foo",
				[]string{
					"example.com.:fixtures/example.com",
					"earth.sun.galaxy.:fixtures/earth.sun.galaxy",
					"myzone.com.:fixtures/myzone.com",
					"sub.myzone.com.:fixtures/sub.myzone.com",
				},
				"1m",
				"",
				dir,
			)
			require.NoError(t, err, "Failed to create config")
			file, err := NewFile(config)
			require.NoError(t, err, "Failed to create file")
			fn(t, file)
		})
	}
}

func testNewFile(t *testing.T, file *File) {
	assert.Len(t, file.Z, 4, "Four zones should be loaded")
	assert.Len(t, file.Z["example.com."].All(), 5, "example.com should have 5 records")
	assert.Len(t, file.Z["earth.sun.galaxy."].All(), 12, "Earth should have 12 records")
	assert.Len(t, file.Z["myzone.com."].All(), 3, "myzone.com should have 3 records")
	assert.Len(t, file.Z["sub.myzone.com."].All(), 1, "sub.myzone.com should have 1 record")
}

func testInsert(t *testing.T, file *File) {
	// Insert a record
	rr, err := dns.NewRR("new.example.com. 3600 IN A")
	require.NoError(t, err, "Failed to create RR")
	file.Insert("example.com.", rr)
	assert.Len(t, file.Z["example.com."].All(), 6, "example.com should have 6 records")
	file.Z["example.com."].Lookup(context.Background(), testRequest(), "example.com.")
}

func testChangeLog(t *testing.T, file *File) {
	// Insert a record
	rr, err := dns.NewRR("new.example.com. 3600 IN A")
	require.NoError(t, err, "Failed to create RR")
	err = file.Insert("example.com.", rr)
	require.NoError(t, err, "Failed to insert RR")
	assert.Len(t, file.Z["example.com."].All(), 6, "example.com should have 6 records")
	// insert another one
	rr, err = dns.NewRR("new2.example.com. 3600 IN A")
	require.NoError(t, err, "Failed to create RR")
	err = file.Insert("example.com.", rr)
	require.NoError(t, err, "Failed to insert RR")
	assert.Len(t, file.Z["example.com."].All(), 7, "example.com should have 7 records")
	// delete one
	rr, err = dns.NewRR("new.example.com. 3600 IN A")
	require.NoError(t, err, "Failed to create RR")
	err = file.Delete("example.com.", rr)
	require.NoError(t, err, "Failed to delete RR")
	assert.Len(t, file.Z["example.com."].All(), 6, "example.com should have 6 records")
	// there should be 3 changes
	l := file.GetChangeLog("example.com.")
	assert.NotNil(t, l, "Change log should not be nil")
	start, err := l.LowestOffset()
	require.NoError(t, err, "Failed to get lowest offset")
	assert.Equal(t, uint64(0), start, "Start should be 0")
	end, err := l.HighestOffset()
	require.NoError(t, err, "Failed to get highest offset")
	assert.Equal(t, uint64(2), end, "End should be 2")

	err = l.Close()
	require.NoError(t, err, "Failed to close change log")

	// Reopen the change log
	newL, err := changelog.NewLog(l.Dir, l.Config)
	require.NoError(t, err, "Failed to create new change log")
	assert.NotNil(t, newL, "Change log should not be nil")
	start, err = newL.LowestOffset()
	require.NoError(t, err, "Failed to get lowest offset")
	assert.Equal(t, uint64(0), start, "Start should be 0")
	end, err = newL.HighestOffset()
	require.NoError(t, err, "Failed to get highest offset")
	assert.Equal(t, uint64(2), end, "End should be 2")
	// Use newL
	file.ChangeLog["example.com."] = newL
	// delete one
	rr, err = dns.NewRR("delete.example.com. 3600 IN A")
	require.NoError(t, err, "Failed to create RR")
	err = file.Delete("example.com.", rr)
	require.NoError(t, err, "Failed to delete RR")
	assert.Len(t, file.Z["example.com."].All(), 6, "example.com should have 6 records")
	// there should be 5 changes
	l = file.GetChangeLog("example.com.")
	assert.NotNil(t, l, "Change log should not be nil")
	start, err = l.LowestOffset()
	require.NoError(t, err, "Failed to get lowest offset")
	assert.Equal(t, uint64(0), start, "Start should be 0")
	end, err = l.HighestOffset()
	require.NoError(t, err, "Failed to get highest offset")
	assert.Equal(t, uint64(3), end, "End should be 3")
	for i := start; i <= end; i++ {
		rec, err := l.Read(i)
		require.NoError(t, err, "Failed to read change log")
		assert.NotNil(t, rec, "Record should not be nil")
		t.Logf("Record: %v", rec)
	}

}

func TestParse(t *testing.T) {
	t.Parallel()
	// Parse fixture file
	fileName := filepath.Join("fixtures", "example.com")
	reader, err := os.Open(filepath.Clean(fileName))
	require.NoError(t, err, "Failed to open fixture file")
	zone, err := Parse(reader, "example.com.", "example.com.", 0)
	require.NoError(t, err, "Failed to parse fixture file")
	require.NotNil(t, zone, "Failed to parse fixture file")
	assert.Len(t, zone.All(), 5, "Failed to parse fixture file")
}

func TestNewFile(t *testing.T) {
	t.Parallel()
	config, err := config.NewConfig("127.0.0.1", "53", "foo", []string{"example.com.:fixtures/example.com"}, "1m", "", "")
	require.NoError(t, err, "Failed to create config")
	file, err := NewFile(config)
	require.NoError(t, err, "Failed to create file")
	require.NotNil(t, file, "Failed to create file")
}

func testRequest() request.Request {
	m := new(dns.Msg)
	m.SetQuestion("new.example.com.", dns.TypeA)
	m.SetEdns0(4096, true)
	return request.Request{W: &test.ResponseWriter{}, Req: m}
}
