// Package casting controls discovered LAN media renderers and exposes a
// capability-scoped playback relay without exposing the main server on the LAN.
package casting

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrDevice = errors.New("discovered renderer is no longer available")
var ErrNetwork = errors.New("no private LAN interface is available")

type Device struct {
	ID, Name         string
	control, service string
	expires          time.Time
}
type Service struct {
	mu        sync.Mutex
	devices   map[string]Device
	relay     *http.Server
	listener  net.Listener
	client    *http.Client
	tickets   map[string]time.Time
	addresses func() []string
}

func New() *Service {
	return &Service{devices: make(map[string]Device), client: &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (s *Service) AllowTicket(ticket string, until time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tickets == nil {
		s.tickets = make(map[string]time.Time)
	}
	for key, expires := range s.tickets {
		if time.Now().After(expires) {
			delete(s.tickets, key)
		}
	}
	s.tickets[ticket] = until
}

func PrivateIP(ip net.IP) bool {
	return ip != nil && ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}
func Addresses() []string {
	interfaces, _ := net.Interfaces()
	set := map[string]bool{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil && PrivateIP(ip) {
				set[ip.String()] = true
			}
		}
	}
	result := make([]string, 0, len(set))
	for ip := range set {
		result = append(result, ip)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := strings.HasPrefix(result[i], "192.168."), strings.HasPrefix(result[j], "192.168.")
		if a != b {
			return a
		}
		return result[i] < result[j]
	})
	return result
}
func (s *Service) Relay(handler http.Handler) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	addresses := Addresses()
	if s.addresses != nil {
		addresses = s.addresses()
	}
	if len(addresses) == 0 {
		return nil, ErrNetwork
	}
	if s.listener == nil {
		listener, err := net.Listen("tcp4", "0.0.0.0:0")
		if err != nil {
			return nil, err
		}
		s.listener = listener
		s.relay = &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: time.Minute, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			ip := net.ParseIP(host)
			if err != nil || !(PrivateIP(ip) || ip.IsLoopback()) {
				http.Error(w, "LAN only", http.StatusForbidden)
				return
			}
			if r.URL.Path != "/playback" && r.URL.Path != "/subtitles" {
				http.NotFound(w, r)
				return
			}
			s.mu.Lock()
			expires := s.tickets[r.URL.Query().Get("ticket")]
			s.mu.Unlock()
			if !expires.After(time.Now()) {
				http.Error(w, "Playback expired", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Range,Content-Type")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Range,Content-Length,Accept-Ranges")
			w.Header().Set("Referrer-Policy", "no-referrer")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			handler.ServeHTTP(w, r)
		})}
		go func() { _ = s.relay.Serve(listener) }()
	}
	port := s.listener.Addr().(*net.TCPAddr).Port
	result := make([]string, 0, len(addresses))
	for _, ip := range addresses {
		result = append(result, fmt.Sprintf("http://%s:%d", ip, port))
	}
	return result, nil
}
func (s *Service) Close(ctx context.Context) error {
	s.mu.Lock()
	server := s.relay
	s.relay = nil
	s.listener = nil
	s.mu.Unlock()
	if server != nil {
		err := server.Shutdown(ctx)
		if err != nil {
			_ = server.Close()
		}
		return err
	}
	return nil
}

func descriptionURL(location string, remote net.IP) (*url.URL, error) {
	parsed, err := url.Parse(location)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Fragment != "" || !PrivateIP(remote) || !remote.Equal(net.ParseIP(parsed.Hostname())) {
		return nil, ErrDevice
	}
	return parsed, nil
}
func (s *Service) Discover(ctx context.Context) ([]Device, error) {
	addresses := Addresses()
	if len(addresses) == 0 {
		return nil, ErrNetwork
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	type target struct {
		location string
		ip       net.IP
	}
	found := make(chan target, 64)
	var group sync.WaitGroup
	for _, address := range addresses {
		group.Add(1)
		go func(address string) {
			defer group.Done()
			socket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(address)})
			if err != nil {
				return
			}
			defer socket.Close()
			_ = socket.SetDeadline(time.Now().Add(2200 * time.Millisecond))
			request := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n\r\n"
			_, _ = socket.WriteToUDP([]byte(request), &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 1900})
			buffer := make([]byte, 8192)
			for {
				n, remote, err := socket.ReadFromUDP(buffer)
				if err != nil {
					return
				}
				response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(buffer[:n])), nil)
				if err != nil {
					continue
				}
				location := response.Header.Get("Location")
				_ = response.Body.Close()
				if _, err = descriptionURL(location, remote.IP); err != nil {
					continue
				}
				select {
				case found <- target{location, remote.IP}:
				default:
				}
				select {
				case <-ctx.Done():
					return
				default:
				}
			}
		}(address)
	}
	go func() { group.Wait(); close(found) }()
	unique := map[string]target{}
	for target := range found {
		if len(unique) < 64 {
			unique[target.location] = target
		}
	}
	devices := []Device{}
	for _, target := range unique {
		if ctx.Err() != nil {
			break
		}
		device, err := s.describe(ctx, target.location, target.ip)
		if err == nil {
			devices = append(devices, device)
		}
	}
	s.mu.Lock()
	for _, device := range devices {
		s.devices[device.ID] = device
	}
	for id, device := range s.devices {
		if time.Now().After(device.expires) {
			delete(s.devices, id)
		}
	}
	s.mu.Unlock()
	sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
	return devices, nil
}

type description struct {
	Device struct {
		Name     string `xml:"friendlyName"`
		Type     string `xml:"deviceType"`
		Services []struct {
			Type    string `xml:"serviceType"`
			Control string `xml:"controlURL"`
		} `xml:"serviceList>service"`
	} `xml:"device"`
}

func (s *Service) describe(ctx context.Context, location string, ip net.IP) (Device, error) {
	origin, err := descriptionURL(location, ip)
	if err != nil {
		return Device{}, err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	response, err := s.client.Do(request)
	if err != nil {
		return Device{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Device{}, ErrDevice
	}
	var data description
	if err = xml.NewDecoder(io.LimitReader(response.Body, 512*1024)).Decode(&data); err != nil {
		return Device{}, err
	}
	if !strings.HasPrefix(data.Device.Type, "urn:schemas-upnp-org:device:MediaRenderer:") {
		return Device{}, ErrDevice
	}
	for _, service := range data.Device.Services {
		if !strings.HasPrefix(service.Type, "urn:schemas-upnp-org:service:AVTransport:") {
			continue
		}
		reference, err := url.Parse(service.Control)
		if err != nil {
			continue
		}
		control := origin.ResolveReference(reference)
		if _, err = descriptionURL(control.String(), ip); err != nil {
			continue
		}
		hash := sha256.Sum256([]byte(control.String()))
		return Device{ID: hex.EncodeToString(hash[:16]), Name: data.Device.Name, control: control.String(), service: service.Type, expires: time.Now().Add(5 * time.Minute)}, nil
	}
	return Device{}, ErrDevice
}
func escape(value string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(value))
	return buffer.String()
}
func (s *Service) Action(ctx context.Context, id, action, arguments string) error {
	s.mu.Lock()
	device, ok := s.devices[id]
	s.mu.Unlock()
	if !ok || !device.expires.After(time.Now()) {
		return ErrDevice
	}
	envelope := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:` + action + ` xmlns:u="` + escape(device.service) + `"><InstanceID>0</InstanceID>` + arguments + `</u:` + action + `></s:Body></s:Envelope>`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, device.control, strings.NewReader(envelope))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "text/xml; charset=utf-8")
	request.Header.Set("SOAPAction", `"`+device.service+`#`+action+`"`)
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("el televisor rechazó %s (HTTP %d)", action, response.StatusCode)
	}
	return nil
}
func (s *Service) Play(ctx context.Context, id, uri, title, mime string) error {
	parsed, err := url.Parse(uri)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return ErrDevice
	}
	metadata := `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"><item id="0" parentID="-1" restricted="1"><dc:title>` + escape(title) + `</dc:title><upnp:class>object.item.videoItem</upnp:class><res protocolInfo="http-get:*:` + escape(mime) + `:*">` + escape(uri) + `</res></item></DIDL-Lite>`
	if err = s.Action(ctx, id, "SetAVTransportURI", "<CurrentURI>"+escape(uri)+"</CurrentURI><CurrentURIMetaData>"+escape(metadata)+"</CurrentURIMetaData>"); err != nil {
		return err
	}
	return s.Control(ctx, id, "play")
}
func (s *Service) Control(ctx context.Context, id, command string) error {
	switch command {
	case "play":
		return s.Action(ctx, id, "Play", "<Speed>1</Speed>")
	case "pause":
		return s.Action(ctx, id, "Pause", "")
	case "stop":
		return s.Action(ctx, id, "Stop", "")
	default:
		return ErrDevice
	}
}
