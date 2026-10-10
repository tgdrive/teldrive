package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml"
)

type localDB struct {
	Port     string
	Password string
}

func (d localDB) dsn() string {
	return "postgres://teldrive:" + url.QueryEscape(d.Password) + "@127.0.0.1:" + d.Port + "/teldrive?sslmode=disable"
}
func (m *manager) databaseInfo() (localDB, error) {
	var d localDB
	b, e := os.ReadFile(filepath.Join(m.dir, "database.json"))
	if e == nil {
		e = json.Unmarshal(b, &d)
	}
	return d, e
}
func (m *manager) unpackDatabase() error {
	if _, e := os.Stat(filepath.Join(m.dir, "pgsql", "share", "postgres.bki")); e == nil {
		return nil
	}
	data, e := assets.ReadFile("bundle/postgres.zip")
	if e != nil {
		return e
	}
	archive, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return e
	}
	root := filepath.Join(m.dir, "pgsql")
	for _, entry := range archive.File {
		p := filepath.Join(root, filepath.FromSlash(entry.Name))
		relative, e := filepath.Rel(root, p)
		if e != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return errors.New("ruta inválida en archivo PostgreSQL")
		}
		if entry.FileInfo().IsDir() {
			if e = os.MkdirAll(p, 0700); e != nil {
				return e
			}
			continue
		}
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		in, e := entry.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return e
		}
		_, e = io.Copy(out, in)
		closeErr := out.Close()
		in.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
func (m *manager) dbCommand(binary string, d localDB, args ...string) *exec.Cmd {
	c := exec.Command(filepath.Join(m.dir, "pgsql", "bin", binary), args...)
	hidden(c)
	c.Dir = m.dir
	c.Env = append(os.Environ(), "PGPASSWORD="+d.Password)
	return c
}
func (m *manager) readyDatabase(d localDB) error {
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		c := m.dbCommand("pg_isready.exe", d, "-h", "127.0.0.1", "-p", d.Port, "-U", "teldrive", "-d", "postgres", "-t", "1")
		if c.Run() == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("postgreSQL no pudo iniciar. Consulta el registro de la base de datos")
}
func (m *manager) startDatabase(d localDB) error {
	m.mu.Lock()
	running := m.jobs["database"] != nil && m.jobs["database"].Running
	m.mu.Unlock()
	if running {
		return m.readyDatabase(d)
	}
	if e := m.start("database", filepath.Join("pgsql", "bin", "postgres.exe"), "-D", filepath.Join(m.dir, "database"), "-p", d.Port, "-h", "127.0.0.1"); e != nil {
		return e
	}
	return m.readyDatabase(d)
}
func (m *manager) stopDatabase() error {
	m.mu.Lock()
	running := m.jobs["database"] != nil && m.jobs["database"].Running
	m.mu.Unlock()
	if !running {
		return nil
	}
	d, e := m.databaseInfo()
	if e != nil {
		return e
	}
	return m.dbCommand("pg_ctl.exe", d, "stop", "-D", filepath.Join(m.dir, "database"), "-m", "fast", "-w", "-t", "20").Run()
}
func (m *manager) setupDatabase() error {
	if !runtimeInstalled() {
		return errors.New("falta el runtime de Microsoft Visual C++. Instálalo con el botón de la interfaz y vuelve a crear la base local")
	}
	m.mu.Lock()
	running := m.jobs["server"] != nil && m.jobs["server"].Running
	m.mu.Unlock()
	if running {
		return errors.New("detén el servidor antes de configurar la base local")
	}
	if e := m.unpackDatabase(); e != nil {
		return e
	}
	d, e := m.databaseInfo()
	if os.IsNotExist(e) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		d.Port = fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
		listener.Close()
		d.Password = random()
		b, _ := json.Marshal(d)
		if e = os.WriteFile(filepath.Join(m.dir, "database.json"), b, 0600); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	dataDir := filepath.Join(m.dir, "database")
	if _, e = os.Stat(filepath.Join(dataDir, "PG_VERSION")); os.IsNotExist(e) {
		pwfile := filepath.Join(m.dir, "database-password.tmp")
		if e = os.WriteFile(pwfile, []byte(d.Password), 0600); e != nil {
			return e
		}
		c := m.dbCommand("initdb.exe", d, "-D", dataDir, "-U", "teldrive", "-A", "scram-sha-256", "--pwfile", pwfile, "-E", "UTF8", "--locale", "C")
		output, initErr := c.CombinedOutput()
		os.Remove(pwfile)
		os.WriteFile(filepath.Join(m.dir, "database-setup.log"), output, 0600)
		if initErr != nil {
			return fmt.Errorf("no se pudo crear PostgreSQL: %s", string(output))
		}
	}
	if e = m.startDatabase(d); e != nil {
		return e
	}
	c := m.dbCommand("psql.exe", d, "-h", "127.0.0.1", "-p", d.Port, "-U", "teldrive", "-d", "postgres", "-tAc", "SELECT 1 FROM pg_database WHERE datname='teldrive'")
	output, e := c.Output()
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(output)) != "1" {
		if output, e = m.dbCommand("createdb.exe", d, "-h", "127.0.0.1", "-p", d.Port, "-U", "teldrive", "teldrive").CombinedOutput(); e != nil {
			return fmt.Errorf("no se pudo crear la base teldrive: %s", output)
		}
	}
	p := filepath.Join(m.dir, "config.toml")
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	values, e := (toml.Parser()).Unmarshal(b)
	if e != nil {
		return e
	}
	db, ok := values["db"].(map[string]any)
	if !ok {
		db = map[string]any{}
		values["db"] = db
	}
	db["data-source"] = d.dsn()
	b, e = (toml.Parser()).Marshal(values)
	if e != nil {
		return e
	}
	return os.WriteFile(p, b, 0600)
}
func (m *manager) startConfiguredDatabase() error {
	d, e := m.databaseInfo()
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	b, e := os.ReadFile(filepath.Join(m.dir, "config.toml"))
	if e != nil {
		return e
	}
	values, e := (toml.Parser()).Unmarshal(b)
	if e != nil {
		return e
	}
	db, ok := values["db"].(map[string]any)
	if ok && db["data-source"] == d.dsn() {
		return m.startDatabase(d)
	}
	return nil
}
