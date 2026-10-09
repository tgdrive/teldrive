// Teldrive Desktop is a separate, local Windows launcher. Licensed under MIT.
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/knadh/koanf/parsers/toml"
	"golang.org/x/sys/windows"
)

//go:embed index.html bundle/*.gz bundle/*.zip bundle/*.json bundle/*.txt
var assets embed.FS

type job struct {
	Cmd     *exec.Cmd
	File    *os.File
	Running bool
	Exit    string
	Log     string
}
type manager struct {
	mu                           sync.Mutex
	operations                   sync.Mutex
	processJob                   windows.Handle
	dir, origin, secret, backend string
	jobs                         map[string]*job
}
type request struct {
	Config      string `json:"config"`
	Action      string `json:"action"`
	Remote      string `json:"remote"`
	Local       string `json:"local"`
	Destination string `json:"destination"`
	Drive       string `json:"drive"`
	DryRun      bool   `json:"dryRun"`
}

func random() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func hidden(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
func (m *manager) extract() error {
	entries, e := assets.ReadDir("bundle")
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".exe") || strings.HasSuffix(entry.Name(), ".msi") {
			continue
		}
		b, e := assets.ReadFile("bundle/" + entry.Name())
		if e != nil {
			return e
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".gz") {
			reader, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				return err
			}
			b, err = io.ReadAll(reader)
			reader.Close()
			if err != nil {
				return err
			}
			name = strings.TrimSuffix(name, ".gz")
		}
		p := filepath.Join(m.dir, name)
		old, _ := os.ReadFile(p)
		if sha256.Sum256(old) != sha256.Sum256(b) {
			if e = os.WriteFile(p, b, 0600); e != nil {
				return e
			}
		}
	}
	return nil
}
func (m *manager) start(name, binary string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j := m.jobs[name]; j != nil && j.Running {
		return errors.New("Ya hay una tarea en ejecución")
	}
	p := filepath.Join(m.dir, name+".log")
	f, e := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	c := exec.Command(filepath.Join(m.dir, binary), args...)
	hidden(c)
	c.Dir = m.dir
	c.Stdout = f
	c.Stderr = f
	if e = c.Start(); e != nil {
		f.Close()
		return e
	}
	if m.processJob != 0 {
		h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
		if err == nil {
			err = windows.AssignProcessToJobObject(m.processJob, h)
			windows.CloseHandle(h)
		}
		if err != nil {
			c.Process.Kill()
			c.Wait()
			f.Close()
			return fmt.Errorf("No se pudo supervisar el proceso: %w", err)
		}
	}
	j := &job{Cmd: c, File: f, Running: true, Log: p}
	m.jobs[name] = j
	go func() {
		e := c.Wait()
		f.Close()
		m.mu.Lock()
		defer m.mu.Unlock()
		j.Running = false
		j.Exit = "Finalizado correctamente"
		if e != nil {
			j.Exit = e.Error()
		}
	}()
	return nil
}
func (m *manager) stop(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[name]
	if j == nil || !j.Running {
		return nil
	}
	return j.Cmd.Process.Kill()
}
func (m *manager) status() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]any{"desktop": true, "directory": m.dir, "winfsp": winfspInstalled(), "backend": m.backend}
	out["runtime"] = runtimeInstalled()
	for _, name := range []string{"server", "rclone", "winfsp", "database"} {
		j := m.jobs[name]
		if j == nil {
			out[name] = map[string]any{"running": false}
			continue
		}
		b, _ := os.ReadFile(j.Log)
		if len(b) > 32000 {
			b = b[len(b)-32000:]
		}
		out[name] = map[string]any{"running": j.Running, "exit": j.Exit, "log": regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(string(b), "")}
	}
	return out
}
func winfspInstalled() bool {
	for _, root := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
		if root != "" {
			if _, e := os.Stat(filepath.Join(root, "WinFsp", "bin", "winfsp-x64.dll")); e == nil {
				return true
			}
		}
	}
	return false
}
func validRemote(s string) bool {
	if len(s) < 2 || len(s) > 41 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func rcloneArgs(q request) ([]string, error) {
	if !validRemote(q.Remote) {
		return nil, errors.New("Nombre de remoto inválido")
	}
	remote := q.Remote + ":" + q.Destination
	args := []string{"--config", "rclone-teldrive.conf", "--stats", "1s", "--stats-one-line", "--log-level", "INFO"}
	switch q.Action {
	case "list":
		args = append(args, "lsd", remote)
	case "upload", "download":
		if !filepath.IsAbs(q.Local) {
			return nil, errors.New("Selecciona una ruta local absoluta")
		}
		if q.Action == "upload" {
			args = append(args, "copy", q.Local, remote)
		} else {
			args = append(args, "copy", remote, q.Local)
		}
		args = append(args, "--transfers", "2")
		if q.DryRun {
			args = append(args, "--dry-run")
		}
	case "mount":
		if len(q.Drive) != 2 || q.Drive[0] < 'D' || q.Drive[0] > 'Z' || q.Drive[1] != ':' {
			return nil, errors.New("Usa una letra de D: a Z:")
		}
		if _, e := os.Stat(q.Drive + "\\"); e == nil {
			return nil, errors.New("La letra de unidad ya está ocupada")
		}
		args = append(args, "mount", q.Remote+":", q.Drive, "--vfs-cache-mode", "writes", "--cache-dir", "rclone-cache")
	default:
		return nil, errors.New("Acción de rclone no permitida")
	}
	return args, nil
}
func (m *manager) api(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Host != strings.TrimPrefix(m.origin, "http://") {
		http.Error(w, "Dirección no permitida", 403)
		return
	}
	c, e := r.Cookie("teldrive_desktop")
	if e != nil || c.Value != m.secret {
		http.Error(w, "Abre el programa desde su ejecutable", 403)
		return
	}
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/desktop/api/status":
			json.NewEncoder(w).Encode(m.status())
			return
		case "/desktop/api/schema":
			b, _ := assets.ReadFile("bundle/settings.json")
			w.Write(b)
			return
		case "/desktop/api/config":
			b, e := os.ReadFile(filepath.Join(m.dir, "config.toml"))
			if e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
			values, e := (toml.Parser()).Unmarshal(b)
			if e != nil {
				http.Error(w, "La configuración TOML no es válida", 500)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"config": string(b), "values": values})
			return
		}
		http.NotFound(w, r)
		return
	}
	if r.Method != "POST" || r.Header.Get("Origin") != m.origin || r.Header.Get("X-Teldrive-Desktop") != "1" {
		http.Error(w, "Petición no permitida", 403)
		return
	}
	var q request
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if e = d.Decode(&q); e != nil {
		http.Error(w, "Petición inválida", 400)
		return
	}
	m.operations.Lock()
	defer m.operations.Unlock()
	switch r.URL.Path {
	case "/desktop/api/config":
		if _, e = (toml.Parser()).Unmarshal([]byte(q.Config)); e == nil {
			m.mu.Lock()
			running := m.jobs["server"] != nil && m.jobs["server"].Running
			m.mu.Unlock()
			if running {
				e = errors.New("Detén el servidor antes de guardar la configuración")
			} else {
				p := filepath.Join(m.dir, "config.toml")
				e = os.WriteFile(p+".tmp", []byte(q.Config), 0600)
				if e == nil {
					e = os.Rename(p+".tmp", p)
				}
			}
		}
	case "/desktop/api/server/start":
		e = m.startConfiguredDatabase()
		if e != nil {
			break
		}
		port := m.backend[strings.LastIndex(m.backend, ":")+1:]
		args := []string{"run", "--config", filepath.Join(m.dir, "config.toml"), "--server-port", port, "--server-bind-address", "127.0.0.1"}
		b, readErr := os.ReadFile(filepath.Join(m.dir, "config.toml"))
		if readErr != nil {
			e = readErr
			break
		}
		values, parseErr := (toml.Parser()).Unmarshal(b)
		if parseErr != nil {
			e = parseErr
			break
		}
		serverValues, _ := values["server"].(map[string]any)
		mediaValues, _ := serverValues["media"].(map[string]any)
		path, _ := mediaValues["ffmpeg-path"].(string)
		if path == "" || path == "ffmpeg" || path == "ffmpeg.exe" {
			args = append(args, "--server-media-ffmpeg-path", filepath.Join(m.dir, "ffmpeg.exe"))
		}
		e = m.start("server", "teldrive.exe", args...)
	case "/desktop/api/server/stop":
		e = m.stop("rclone")
		if e == nil {
			e = m.stop("server")
		}
	case "/desktop/api/database/setup":
		e = m.setupDatabase()
	case "/desktop/api/runtime/install":
		e = m.installRuntime()
	case "/desktop/api/rclone/start":
		if strings.ContainsAny(q.Config, "\x00") || !strings.HasPrefix(q.Config, "["+q.Remote+"]\n") || !strings.Contains(q.Config, "\ntype = teldrive\n") {
			e = errors.New("Configuración de rclone inválida")
			break
		}
		// Accept only the generated Teldrive INI; prevent arbitrary backends and injected sections.
		allowed := map[string]bool{"type": true, "api_host": true, "access_token": true, "chunk_size": true, "upload_concurrency": true, "page_size": true, "hash_enabled": true, "encrypt_files": true, "channel_id": true}
		seen := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(q.Config), "\n")[1:] {
			kv := strings.SplitN(line, "=", 2)
			if len(kv) != 2 || !allowed[strings.TrimSpace(kv[0])] || seen[strings.TrimSpace(kv[0])] || strings.ContainsAny(kv[1], "\r\n") {
				e = errors.New("La configuración contiene opciones no permitidas")
				break
			}
			key, value := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
			seen[key] = true
			if key == "type" && value != "teldrive" {
				e = errors.New("Solo se permite el backend Teldrive")
				break
			}
		}
		if e != nil {
			break
		}
		var args []string
		args, e = rcloneArgs(q)
		if e != nil {
			break
		}
		if q.Action == "mount" && !winfspInstalled() {
			e = errors.New("Instala WinFsp con el botón de esta interfaz antes de montar")
			break
		}
		m.mu.Lock()
		running := m.jobs["rclone"] != nil && m.jobs["rclone"].Running
		m.mu.Unlock()
		if running {
			e = errors.New("Detén la tarea actual antes de iniciar otra")
			break
		}
		e = os.WriteFile(filepath.Join(m.dir, "rclone-teldrive.conf"), []byte(q.Config), 0600)
		if e == nil {
			e = m.start("rclone", "rclone.exe", args...)
		}
	case "/desktop/api/rclone/stop":
		e = m.stop("rclone")
	case "/desktop/api/winfsp/install":
		// Windows Installer shows its own elevation and installation UI; do not bypass UAC.
		c := exec.Command("msiexec.exe", "/i", filepath.Join(m.dir, "winfsp-2.1.25156.msi"), "/norestart")
		hidden(c)
		e = c.Start()
		if e == nil {
			go c.Wait()
		}
	case "/desktop/api/quit":
		m.stop("rclone")
		m.stop("server")
		if e = m.stopDatabase(); e != nil {
			break
		}
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		go func() { time.Sleep(time.Second); os.Exit(0) }()
		return
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
func (m *manager) serve(w http.ResponseWriter, r *http.Request, proxy *httputil.ReverseProxy) {
	if r.Host != strings.TrimPrefix(m.origin, "http://") {
		http.Error(w, "Dirección no permitida", 403)
		return
	}
	if r.URL.Path == "/desktop" || r.URL.Path == "/desktop/" {
		if key := r.URL.Query().Get("key"); key != "" {
			if key != m.secret {
				http.Error(w, "Clave inválida", 403)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "teldrive_desktop", Value: m.secret, Path: "/desktop", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/desktop", http.StatusSeeOther)
			return
		}
		c, e := r.Cookie("teldrive_desktop")
		if e != nil || c.Value != m.secret {
			http.Error(w, "Abre Teldrive Desktop.exe para acceder", 403)
			return
		}
		b, _ := assets.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		w.Write(b)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/desktop/api/") {
		m.api(w, r)
		return
	}
	proxy.ServeHTTP(w, r)
}
func openApp(address string) error {
	profile := filepath.Join(os.Getenv("LOCALAPPDATA"), "Teldrive Desktop", "WebProfile")
	for _, root := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
		p := filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe")
		if _, e := os.Stat(p); e == nil {
			c := exec.Command(p, "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check", "--new-window", "--app="+address)
			if err := c.Start(); err == nil {
				go c.Wait()
				return nil
			} else {
				log.Printf("No se pudo abrir la ventana de Edge: %v", err)
			}
		}
	}
	verb, _ := windows.UTF16PtrFromString("open")
	target, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}

func showStartupError(err error) {
	log.Print(err)
	title, _ := windows.UTF16PtrFromString("Teldrive Desktop")
	message, _ := windows.UTF16PtrFromString("No se pudo abrir Teldrive Desktop.\n\n" + err.Error() + "\n\nRegistro: " + filepath.Join(os.Getenv("LOCALAPPDATA"), "Teldrive Desktop", "desktop.log"))
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}
func main() {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Teldrive Desktop")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return
	}
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf("Local\\TeldriveDesktop-%x", sha256.Sum256([]byte(dir))))
	mutex, mutexErr := windows.CreateMutex(nil, false, name)
	if errors.Is(mutexErr, windows.ERROR_ALREADY_EXISTS) {
		defer windows.CloseHandle(mutex)
		for i := 0; i < 50; i++ {
			if b, err := os.ReadFile(filepath.Join(dir, "window.url")); err == nil {
				if err = openApp(string(b)); err != nil {
					showStartupError(err)
				}
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		showStartupError(errors.New("La instancia anterior todavía no publicó su dirección. Vuelve a abrir el ejecutable en unos segundos"))
		return
	}
	if mutexErr != nil {
		showStartupError(mutexErr)
		return
	}
	defer windows.CloseHandle(mutex)
	f, e := os.OpenFile(filepath.Join(dir, "desktop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return
	}
	defer f.Close()
	log.SetOutput(f)
	m := &manager{dir: dir, secret: random(), jobs: map[string]*job{}}
	m.processJob, e = windows.CreateJobObject(nil, nil)
	if e != nil {
		log.Print(e)
		return
	}
	defer windows.CloseHandle(m.processJob)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(m.processJob, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		log.Print(e)
		return
	}
	if e = m.extract(); e != nil {
		log.Print(e)
		return
	}
	p := filepath.Join(dir, "config.toml")
	if _, e = os.Stat(p); os.IsNotExist(e) {
		initial := "[db]\ndata-source = \"postgres://postgres:CAMBIAR@127.0.0.1:5432/teldrive?sslmode=disable\"\n\n[jwt]\nsecret = \"" + random() + "\"\n\n[server.media]\nffmpeg-path = \"ffmpeg.exe\"\n"
		if e = os.WriteFile(p, []byte(initial), 0600); e != nil {
			log.Print(e)
			return
		}
	}
	address := "127.0.0.1:0"
	if b, err := os.ReadFile(filepath.Join(dir, "port.txt")); err == nil {
		port := strings.TrimSpace(string(b))
		if _, err = net.LookupPort("tcp", port); err == nil {
			address = net.JoinHostPort("127.0.0.1", port)
		}
	}
	listener, e := net.Listen("tcp", address)
	if e != nil {
		listener, e = net.Listen("tcp", "127.0.0.1:0")
	}
	if e != nil {
		log.Print(e)
		return
	}
	m.origin = "http://" + listener.Addr().String()
	os.WriteFile(filepath.Join(dir, "port.txt"), []byte(fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)), 0600)
	backend, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		log.Print(e)
		return
	}
	m.backend = "http://" + backend.Addr().String()
	backend.Close()
	u, _ := url.Parse(m.backend)
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/desktop", 303)
			return
		}
		http.Error(w, "El servidor está detenido. Abre /desktop para configurarlo e iniciarlo.", 503)
	}
	// Preserve the browser's original Host/Origin for cookie and CSRF checks on proxied APIs.
	proxy.ModifyResponse = func(resp *http.Response) error { resp.Header.Del("Access-Control-Allow-Origin"); return nil }
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.serve(w, r, proxy) }), ReadHeaderTimeout: 10 * time.Second}
	if len(os.Args) > 1 && os.Args[1] == "--self-test" {
		b, _ := json.Marshal(m.status())
		fmt.Println(string(b))
		return
	}
	windowURL := m.origin + "/desktop?key=" + m.secret
	if e = os.WriteFile(filepath.Join(dir, "window.url"), []byte(windowURL), 0600); e != nil {
		log.Print(e)
		return
	}
	if !(len(os.Args) > 1 && os.Args[1] == "--background") {
		if e = openApp(windowURL); e != nil {
			showStartupError(e)
		}
	}
	log.Printf("Interfaz local: %s/desktop", m.origin)
	if e = server.Serve(listener); e != nil {
		log.Print(e)
	}
}
