// START: intro
package changelog

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestLog(t *testing.T) {
	for scenario, fn := range map[string]func(
		t *testing.T, log *Log,
	){
		"append and read a record succeeds": testAppendRead,
		"offset out of range error":         testOutOfRangeErr,
		"init with existing segments":       testInitExisting,
		"reader":                            testReader,
		"truncate":                          testTruncate,
	} {
		t.Run(scenario, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "store-test")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			c := Config{}
			c.Segment.MaxStoreBytes = 32
			log, err := NewLog(dir, c)
			require.NoError(t, err)

			fn(t, log)
		})
	}
}

// END: intro

// START: append_read
func testAppendRead(t *testing.T, log *Log) {
	rec := &dns.A{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		A: net.ParseIP("127.0.0.1"),
	}
	append := &Record{
		Type:   TypeInsert,
		Record: rec.String(),
	}
	off, err := log.Append(append)
	require.NoError(t, err)
	require.Equal(t, uint64(0), off)

	read, err := log.Read(off)
	require.NoError(t, err)
	require.Equal(t, append.Record, read.Record)
}

// END: append_read

// START: out_of_range
func testOutOfRangeErr(t *testing.T, log *Log) {
	read, err := log.Read(1)
	require.Nil(t, read)
	require.Error(t, err)
}

// END: out_of_range

// START: init_existing
func testInitExisting(t *testing.T, o *Log) {
	rec := &dns.A{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		A: net.ParseIP("127.0.0.1"),
	}
	append := &Record{
		Type:   TypeInsert,
		Record: rec.String(),
	}
	for i := 0; i < 3; i++ {
		_, err := o.Append(append)
		require.NoError(t, err)
	}
	require.NoError(t, o.Close())

	off, err := o.LowestOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(0), off)
	off, err = o.HighestOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(2), off)

	n, err := NewLog(o.Dir, o.Config)
	require.NoError(t, err)

	off, err = n.LowestOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(0), off)
	off, err = n.HighestOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(2), off)
}

// END: init_existing

// START: reader
func testReader(t *testing.T, log *Log) {
	rec := &dns.A{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		A: net.ParseIP("127.0.0.1"),
	}
	append := &Record{
		Type:   TypeInsert,
		Record: rec.String(),
	}
	off, err := log.Append(append)
	require.NoError(t, err)
	require.Equal(t, uint64(0), off)

	reader := log.Reader()
	b, err := ioutil.ReadAll(reader)
	require.NoError(t, err)

	read := &Record{}
	err = json.Unmarshal(b[lenWidth:], read)
	require.NoError(t, err)
	require.Equal(t, append.Record, read.Record)
}

// END: reader

// START: truncate
func testTruncate(t *testing.T, log *Log) {
	rec := &dns.A{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		A: net.ParseIP("127.0.0.1"),
	}
	append := &Record{
		Type:   TypeInsert,
		Record: rec.String(),
	}
	for i := 0; i < 3; i++ {
		_, err := log.Append(append)
		require.NoError(t, err)
	}

	err := log.Truncate(1)
	require.NoError(t, err)

	_, err = log.Read(0)
	require.Error(t, err)
}

// END: truncate
