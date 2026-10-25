package zaglushka

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// connString turns the 1C form of a database server, «host port=5432» or
// «host:5432», into a PostgreSQL connection string.
func connString(server, db, user, pwd string) string {
	host, port := server, "5432"
	if h, p, ok := strings.Cut(server, " port="); ok {
		host, port = h, p
	} else if h, p, ok := strings.Cut(server, ":"); ok {
		host, port = h, p
	}
	u := url.URL{Scheme: "postgres", User: url.UserPassword(user, pwd), Host: host + ":" + port, Path: "/" + db}
	q := u.Query()
	q.Set("sslmode", "prefer")
	q.Set("connect_timeout", "10")
	u.RawQuery = q.Encode()
	return u.String()
}

func connect(server, db, user, pwd string) (*pgx.Conn, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	c, err := pgx.Connect(ctx, connString(server, db, user, pwd))
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("Ошибка СУБД:\nне удалось подключиться к серверу баз данных %s: %v", server, err)
	}
	return c, ctx, cancel, nil
}

func createDatabase(ib Infobase) error {
	c, ctx, cancel, err := connect(ib.DBServer, "postgres", ib.DBUser, ib.DBPwd)
	if err != nil {
		return err
	}
	defer cancel()
	defer c.Close(ctx)
	var exists bool
	if err := c.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", ib.DBName).Scan(&exists); err != nil {
		return fmt.Errorf("Ошибка СУБД:\n%v", err)
	}
	if !exists {
		if _, err := c.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{ib.DBName}.Sanitize()); err != nil {
			return fmt.Errorf("Ошибка СУБД:\n%v", err)
		}
	}
	return ensureSchema(ib)
}

func dropDatabase(ib Infobase) error {
	c, ctx, cancel, err := connect(ib.DBServer, "postgres", ib.DBUser, ib.DBPwd)
	if err != nil {
		return err
	}
	defer cancel()
	defer c.Close(ctx)
	_, err = c.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{ib.DBName}.Sanitize()+" WITH (FORCE)")
	if err != nil {
		return fmt.Errorf("Ошибка СУБД:\n%v", err)
	}
	return nil
}

// ensureSchema makes the tables the stand-in keeps an infobase in: its
// configuration and the configuration waiting to be applied.
func ensureSchema(ib Infobase) error {
	c, ctx, cancel, err := connect(ib.DBServer, ib.DBName, ib.DBUser, ib.DBPwd)
	if err != nil {
		return err
	}
	defer cancel()
	defer c.Close(ctx)
	_, err = c.Exec(ctx, `
CREATE TABLE IF NOT EXISTS config (
  kind text PRIMARY KEY,            -- 'main' or 'pending'
  name text NOT NULL,
  synonym text NOT NULL DEFAULT '',
  version text NOT NULL,
  vendor text NOT NULL DEFAULT '',
  loaded_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS extensions (
  name text PRIMARY KEY,
  version text NOT NULL,
  loaded_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS data (
  key text PRIMARY KEY,
  value text NOT NULL
);`)
	if err != nil {
		return fmt.Errorf("Ошибка СУБД:\n%v", err)
	}
	return nil
}

// Configuration is what the stand-in reads from a .cf file: a text file
// of «Ключ: значение» lines, Имя, Синоним, Версия, Поставщик.
type Configuration struct {
	Name, Synonym, Version, Vendor string
}

func parseCF(data []byte) (Configuration, error) {
	var c Configuration
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Имя", "Name":
			c.Name = v
		case "Синоним", "Synonym":
			c.Synonym = v
		case "Версия", "Version":
			c.Version = v
		case "Поставщик", "Vendor":
			c.Vendor = v
		}
	}
	if c.Name == "" || c.Version == "" {
		return c, fmt.Errorf("Файл не является файлом конфигурации заглушки: нужны строки «Имя:» и «Версия:» (настоящий .cf загружает настоящая платформа)")
	}
	return c, nil
}

// ReadConfig returns the configuration of an infobase, «main» or «pending».
func ReadConfig(ib Infobase, kind string) (Configuration, bool, error) {
	c, ctx, cancel, err := connect(ib.DBServer, ib.DBName, ib.DBUser, ib.DBPwd)
	if err != nil {
		return Configuration{}, false, err
	}
	defer cancel()
	defer c.Close(ctx)
	var cf Configuration
	err = c.QueryRow(ctx, "SELECT name, synonym, version, vendor FROM config WHERE kind = $1", kind).
		Scan(&cf.Name, &cf.Synonym, &cf.Version, &cf.Vendor)
	if err == pgx.ErrNoRows {
		return cf, false, nil
	}
	if err != nil {
		return cf, false, fmt.Errorf("Ошибка СУБД:\n%v", err)
	}
	return cf, true, nil
}

func writeConfig(ib Infobase, kind string, cf Configuration) error {
	c, ctx, cancel, err := connect(ib.DBServer, ib.DBName, ib.DBUser, ib.DBPwd)
	if err != nil {
		return err
	}
	defer cancel()
	defer c.Close(ctx)
	_, err = c.Exec(ctx, `INSERT INTO config (kind, name, synonym, version, vendor) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (kind) DO UPDATE SET name = $2, synonym = $3, version = $4, vendor = $5, loaded_at = now()`,
		kind, cf.Name, cf.Synonym, cf.Version, cf.Vendor)
	if err != nil {
		return fmt.Errorf("Ошибка СУБД:\n%v", err)
	}
	return nil
}

func applyConfig(ib Infobase) (Configuration, error) {
	cf, ok, err := ReadConfig(ib, "pending")
	if err != nil {
		return cf, err
	}
	if !ok {
		return cf, fmt.Errorf("Конфигурация базы данных соответствует основной конфигурации: обновлять нечего")
	}
	if err := writeConfig(ib, "main", cf); err != nil {
		return cf, err
	}
	c, ctx, cancel, err := connect(ib.DBServer, ib.DBName, ib.DBUser, ib.DBPwd)
	if err != nil {
		return cf, err
	}
	defer cancel()
	defer c.Close(ctx)
	_, err = c.Exec(ctx, "DELETE FROM config WHERE kind = 'pending'")
	return cf, err
}
