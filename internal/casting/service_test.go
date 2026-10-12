package casting

import (
	"context"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDiscoveryRejectsExternalAndMetadataURLs(t *testing.T) {
	for _, location := range []string{"http://169.254.169.254/latest/meta-data", "http://127.0.0.1:8000/control", "http://example.org/control", "http://192.168.2.4/control", "https://192.168.2.3/control", "http://user:password@192.168.2.3/control"} {
		if _, err := descriptionURL(location, net.ParseIP("192.168.2.3")); err == nil {
			t.Fatalf("accepted invalid description: %s", location)
		}
	}
	if _, err := descriptionURL("http://192.168.2.3:8000/device.xml", net.ParseIP("192.168.2.3")); err != nil {
		t.Fatal(err)
	}
}
func TestDLNACommandsAndEscapedMetadata(t *testing.T) {
	var actions []string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actions = append(actions, r.Header.Get("SOAPAction"))
		data, _ := io.ReadAll(r.Body)
		decoder := xml.NewDecoder(strings.NewReader(string(data)))
		for {
			if _, err := decoder.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Error(err)
				break
			}
		}
		if strings.Contains(r.Header.Get("SOAPAction"), "SetAVTransportURI") && !strings.Contains(string(data), "a=1&amp;b=2") {
			t.Error("unescaped media URI")
		}
		w.WriteHeader(200)
	}))
	defer receiver.Close()
	service := New()
	service.devices["tv"] = Device{ID: "tv", Name: "Televisor", control: receiver.URL, service: "urn:schemas-upnp-org:service:AVTransport:1", expires: time.Now().Add(time.Minute)}
	if err := service.Play(context.Background(), "tv", "http://192.168.1.4:8090/playback?a=1&b=2", "Título <privado> & vídeo", "video/mp4"); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"pause", "play", "stop"} {
		if err := service.Control(context.Background(), "tv", command); err != nil {
			t.Fatal(err)
		}
	}
	if len(actions) != 5 || !strings.Contains(actions[0], "SetAVTransportURI") || !strings.Contains(actions[1], "#Play") {
		t.Fatal(actions)
	}
	if err := service.Control(context.Background(), "unknown", "play"); err == nil {
		t.Fatal("unknown renderer accepted")
	}
}
func TestRelayExposesOnlyGrantedMedia(t *testing.T) {
	service := New()
	service.addresses = func() []string { return []string{"127.0.0.1"} }
	addresses, err := service.Relay(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("authorized video")) }))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())
	for _, path := range []string{"/config.toml", "/playback?ticket=missing"} {
		r, err := http.Get(addresses[0] + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode == 200 {
			t.Fatalf("exposed %s", path)
		}
	}
	service.AllowTicket("valid", time.Now().Add(time.Minute))
	r, err := http.Get(addresses[0] + "/playback?ticket=valid")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	data, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || string(data) != "authorized video" {
		t.Fatal(r.StatusCode, string(data))
	}
	service.AllowTicket("expired", time.Now().Add(-time.Minute))
	r, err = http.Get(addresses[0] + "/playback?ticket=expired")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal("expired ticket accepted")
	}
}
