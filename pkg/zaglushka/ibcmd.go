package zaglushka

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Ibcmd runs a command of ibcmd: infobases without the Designer.
func Ibcmd(args []string, out io.Writer) error {
	a := parseRac(args)
	w := a.words
	if len(w) < 2 || w[0] != "infobase" {
		return fmt.Errorf("Использование: ibcmd infobase create|dump|restore|config ...")
	}
	ib := Infobase{
		DBMS: a.opts["dbms"], DBServer: a.opts["db-server"], DBName: a.opts["db-name"],
		DBUser: a.opts["db-user"], DBPwd: a.opts["db-pwd"],
	}
	if ib.DBServer == "" || ib.DBName == "" {
		return fmt.Errorf("Не указаны параметры базы данных: --db-server, --db-name")
	}
	if !strings.EqualFold(ib.DBMS, "PostgreSQL") {
		return fmt.Errorf("Заглушка работает только с PostgreSQL, а указано --dbms=%s", ib.DBMS)
	}
	file := ""
	if len(w) > 2 {
		file = w[len(w)-1]
	}
	switch w[1] {
	case "create":
		fmt.Fprintln(out, "Создание информационной базы...")
		if err := createDatabase(ib); err != nil {
			return err
		}
		if dt, ok := a.str("restore"); ok {
			return restore(ib, dt, out)
		}
		if cf, ok := a.str("load"); ok {
			if err := loadCF(ib, cf, out); err != nil {
				return err
			}
			if _, ok := a.str("apply"); ok {
				if _, err := applyConfig(ib); err != nil {
					return err
				}
			}
		}
		fmt.Fprintln(out, "Создание информационной базы успешно завершено")
	case "dump":
		if file == "" || file == "dump" {
			return fmt.Errorf("Не указан файл выгрузки")
		}
		return dump(ib, file, out)
	case "restore":
		if file == "" || file == "restore" {
			return fmt.Errorf("Не указан файл выгрузки")
		}
		if _, ok := a.str("create-database"); ok {
			if err := createDatabase(ib); err != nil {
				return err
			}
		}
		return restore(ib, file, out)
	case "config":
		if len(w) < 3 {
			return fmt.Errorf("Использование: ibcmd infobase config load|apply|save ...")
		}
		switch w[2] {
		case "load":
			if ext, ok := a.str("extension"); ok {
				return loadExtension(ib, ext, file, out)
			}
			return loadCF(ib, file, out)
		case "apply":
			fmt.Fprintln(out, "Обновление конфигурации базы данных...")
			cf, err := applyConfig(ib)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Обновление конфигурации базы данных успешно завершено: %s %s\n", cf.Name, cf.Version)
		case "save":
			cf, ok, err := ReadConfig(ib, "main")
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("Конфигурация не загружена")
			}
			return os.WriteFile(file, []byte(fmt.Sprintf("Имя: %s\nСиноним: %s\nВерсия: %s\nПоставщик: %s\n", cf.Name, cf.Synonym, cf.Version, cf.Vendor)), 0o644)
		default:
			return fmt.Errorf("Неизвестная команда: config %s", w[2])
		}
	default:
		return fmt.Errorf("Неизвестная команда: infobase %s", w[1])
	}
	return nil
}

func loadCF(ib Infobase, file string, out io.Writer) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("Ошибка открытия файла конфигурации: %v", err)
	}
	cf, err := parseCF(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Загрузка конфигурации %s %s...\n", cf.Name, cf.Version)
	if err := ensureSchema(ib); err != nil {
		return err
	}
	return writeConfig(ib, "pending", cf)
}

func loadExtension(ib Infobase, name, file string, out io.Writer) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("Ошибка открытия файла расширения: %v", err)
	}
	cf, err := parseCF(data)
	if err != nil {
		return err
	}
	c, ctx, cancel, err := connect(ib.DBServer, ib.DBName, ib.DBUser, ib.DBPwd)
	if err != nil {
		return err
	}
	defer cancel()
	defer c.Close(ctx)
	_, err = c.Exec(ctx, `INSERT INTO extensions (name, version) VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET version = $2, loaded_at = now()`, name, cf.Version)
	if err == nil {
		fmt.Fprintf(out, "Расширение %s %s загружено\n", name, cf.Version)
	}
	return err
}

// dumpFile is the stand-in's .dt: the tables of the infobase as JSON.
type dumpFile struct {
	Format     string            `json:"format"`
	Config     *Configuration    `json:"config"`
	Extensions map[string]string `json:"extensions"`
	Data       map[string]string `json:"data"`
}

func dump(ib Infobase, file string, out io.Writer) error {
	fmt.Fprintln(out, "Выгрузка информационной базы...")
	d := dumpFile{Format: "assnetes-zaglushka-dt/1", Extensions: map[string]string{}, Data: map[string]string{}}
	if cf, ok, err := ReadConfig(ib, "main"); err != nil {
		return err
	} else if ok {
		d.Config = &cf
	}
	c, ctx, cancel, err := connect(ib.DBServer, ib.DBName, ib.DBUser, ib.DBPwd)
	if err != nil {
		return err
	}
	defer cancel()
	defer c.Close(ctx)
	for table, into := range map[string]map[string]string{"extensions": d.Extensions, "data": d.Data} {
		col := "value"
		if table == "extensions" {
			col = "version"
		}
		rows, err := c.Query(ctx, fmt.Sprintf("SELECT %s, %s FROM %s", map[string]string{"extensions": "name", "data": "key"}[table], col, table))
		if err != nil {
			return fmt.Errorf("Ошибка СУБД:\n%v", err)
		}
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				rows.Close()
				return err
			}
			into[k] = v
		}
		rows.Close()
	}
	data, _ := json.MarshalIndent(d, "", "  ")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "Выгрузка информационной базы успешно завершена")
	return nil
}

func restore(ib Infobase, file string, out io.Writer) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("Ошибка открытия файла выгрузки: %v", err)
	}
	var d dumpFile
	if err := json.Unmarshal(data, &d); err != nil || d.Format == "" {
		return fmt.Errorf("Файл не является выгрузкой информационной базы заглушки")
	}
	fmt.Fprintln(out, "Загрузка информационной базы...")
	if err := ensureSchema(ib); err != nil {
		return err
	}
	c, ctx, cancel, err := connect(ib.DBServer, ib.DBName, ib.DBUser, ib.DBPwd)
	if err != nil {
		return err
	}
	defer cancel()
	defer c.Close(ctx)
	if _, err := c.Exec(ctx, "TRUNCATE config, extensions, data"); err != nil {
		return err
	}
	if d.Config != nil {
		if err := writeConfig(ib, "main", *d.Config); err != nil {
			return err
		}
	}
	for k, v := range d.Extensions {
		if _, err := c.Exec(ctx, "INSERT INTO extensions (name, version) VALUES ($1, $2)", k, v); err != nil {
			return err
		}
	}
	for k, v := range d.Data {
		if _, err := c.Exec(ctx, "INSERT INTO data (key, value) VALUES ($1, $2)", k, v); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "Загрузка информационной базы успешно завершена")
	return nil
}
