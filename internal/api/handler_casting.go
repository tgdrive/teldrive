package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/casting"
)

func (h *Handler) castingAccess(ctx context.Context) error {
	if !HasAdminRole(ctx) {
		return problem(403, "casting_owner_required", "La búsqueda de dispositivos en la red del servidor requiere una cuenta administradora", nil)
	}
	if h.Casting == nil {
		return problem(503, "casting_unavailable", "Transmisión no disponible en este servidor", nil)
	}
	return nil
}
func (h *Handler) CreateCastingRelay(ctx context.Context, req *gen.CreateCastingRelayReq) (gen.CreateCastingRelayRes, error) {
	if err := h.castingAccess(ctx); err != nil {
		return nil, err
	}
	grant, _, err := h.resolvePlayback(ctx, req.Ticket)
	if err != nil {
		return nil, err
	}
	actor, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if grant.actorID != actor {
		return nil, problem(403, "casting_ticket_owner", "Abre el vídeo desde tu cuenta para transmitirlo desde este servidor", nil)
	}
	addresses, err := h.Casting.Relay(http.HandlerFunc(h.serveCastingMedia))
	if err != nil {
		return nil, problem(503, "casting_network", "No se encontró una interfaz de red local disponible", err)
	}
	h.Casting.AllowTicket(req.Ticket, grant.expires)
	return &gen.CastingRelay{Addresses: addresses, ExpiresAt: grant.expires}, nil
}
func (h *Handler) ListCastingDevices(ctx context.Context) (gen.ListCastingDevicesRes, error) {
	if err := h.castingAccess(ctx); err != nil {
		return nil, err
	}
	devices, err := h.Casting.Discover(ctx)
	if err != nil {
		return nil, problem(503, "casting_discovery", "No se pudieron buscar televisores en la red del servidor", err)
	}
	result := &gen.CastingDevices{Devices: []gen.CastingDevice{}}
	for _, device := range devices {
		result.Devices = append(result.Devices, gen.CastingDevice{ID: device.ID, Name: device.Name, Protocol: gen.CastingDeviceProtocolDlna})
	}
	return result, nil
}
func playbackAddress(base, ticket, mode string, start float64, audio, subtitle gen.OptInt32) (string, error) {
	origin, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || len(base) > 1024 {
		return "", problem(400, "casting_address", "Introduce una dirección HTTP o HTTPS accesible para el televisor", nil)
	}
	if origin.Path != "" && origin.Path != "/api/v1" {
		return "", problem(400, "casting_address", "La dirección debe terminar en /api/v1 para el servidor principal o no incluir ruta para el enlace de red local", nil)
	}
	origin.Path += "/playback"
	query := url.Values{"ticket": {ticket}, "mode": {mode}, "start": {strconv.FormatFloat(start, 'f', 3, 64)}}
	if value, ok := audio.Get(); ok {
		query.Set("audio", strconv.Itoa(int(value)))
	}
	if value, ok := subtitle.Get(); ok {
		query.Set("subtitle", strconv.Itoa(int(value)))
	}
	origin.RawQuery = query.Encode()
	return origin.String(), nil
}
func (h *Handler) PlayOnCastingDevice(ctx context.Context, req *gen.CastingStart, params gen.PlayOnCastingDeviceParams) (gen.PlayOnCastingDeviceRes, error) {
	if err := h.castingAccess(ctx); err != nil {
		return nil, err
	}
	grant, file, err := h.resolvePlayback(ctx, req.Ticket)
	if err != nil {
		return nil, err
	}
	actor, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if actor != grant.actorID {
		return nil, problem(403, "casting_ticket_owner", "La sesión de reproducción pertenece a otra cuenta", nil)
	}
	if _, err = h.selectMediaTracks(ctx, grant, file, req.Audio, req.Subtitle); err != nil {
		return nil, err
	}
	uri, err := playbackAddress(req.ServerUrl, req.Ticket, string(req.Mode), req.Start.Or(0), req.Audio, req.Subtitle)
	if err != nil {
		return nil, err
	}
	mime := file.MimeType.String
	if req.Mode == gen.CastingStartModeVideo {
		if !h.Media.Available() {
			return nil, problem(503, "media_unavailable", "La transmisión compatible necesita FFmpeg en el servidor", nil)
		}
		mime = "video/mp4"
	}
	if err = h.Casting.Play(ctx, params.DeviceId, uri, file.Name, mime); err != nil {
		return nil, castDeviceError(err)
	}
	return &gen.PlayOnCastingDeviceNoContent{}, nil
}
func (h *Handler) ControlCastingDevice(ctx context.Context, req *gen.ControlCastingDeviceReq, params gen.ControlCastingDeviceParams) (gen.ControlCastingDeviceRes, error) {
	if err := h.castingAccess(ctx); err != nil {
		return nil, err
	}
	if err := h.Casting.Control(ctx, params.DeviceId, string(req.Command)); err != nil {
		return nil, castDeviceError(err)
	}
	return &gen.ControlCastingDeviceNoContent{}, nil
}
func castDeviceError(err error) error {
	if errors.Is(err, casting.ErrDevice) {
		return problem(404, "casting_device_expired", "Vuelve a buscar dispositivos; el televisor ya no está disponible", err)
	}
	return problem(503, "casting_device_failed", "El televisor no pudo iniciar la reproducción. Comprueba la red y prueba el modo compatible", err)
}

func (h *Handler) serveCastingMedia(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	ticket := query.Get("ticket")
	grant, file, err := h.resolvePlayback(r.Context(), ticket)
	if err != nil {
		http.Error(w, "Playback unavailable", http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodHead {
		mime := file.MimeType.String
		if query.Get("mode") == "video" {
			mime = "video/mp4"
		} else {
			w.Header().Set("Content-Length", fmt.Sprint(file.Size.Int64))
			w.Header().Set("Accept-Ranges", "bytes")
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		return
	}
	_ = grant
	if r.URL.Path == "/subtitles" {
		index, err := strconv.ParseInt(query.Get("track"), 10, 32)
		if err != nil || index < 0 || index > 1024 {
			http.Error(w, "Invalid track", 400)
			return
		}
		err = NewRawHandler(h).GetPlaybackSubtitles(r.Context(), gen.GetPlaybackSubtitlesParams{Ticket: ticket, Track: int32(index)}, w)
		if err != nil {
			http.Error(w, "Subtitles unavailable", 422)
		}
		return
	}
	mode := query.Get("mode")
	if mode == "" {
		mode = "original"
	}
	if mode != "original" && mode != "video" {
		http.Error(w, "Invalid mode", 400)
		return
	}
	params := gen.StreamPlaybackParams{Ticket: ticket, Mode: gen.NewOptStreamPlaybackMode(gen.StreamPlaybackMode(mode))}
	if value := r.Header.Get("Range"); value != "" {
		params.Range = gen.NewOptString(value)
	}
	if value := query.Get("start"); value != "" {
		start, err := strconv.ParseFloat(value, 64)
		if err != nil || start < 0 || start > 86400 {
			http.Error(w, "Invalid start", 400)
			return
		}
		params.Start = gen.NewOptFloat64(start)
	}
	for _, name := range []string{"audio", "subtitle"} {
		if raw := query.Get(name); raw != "" {
			index, err := strconv.ParseInt(raw, 10, 32)
			if err != nil || index < 0 || index > 1024 {
				http.Error(w, "Invalid track", 400)
				return
			}
			if name == "audio" {
				params.Audio = gen.NewOptInt32(int32(index))
			} else {
				params.Subtitle = gen.NewOptInt32(int32(index))
			}
		}
	}
	if err = NewRawHandler(h).StreamPlayback(r.Context(), params, w); err != nil {
		http.Error(w, "Playback unavailable", 503)
	}
}
