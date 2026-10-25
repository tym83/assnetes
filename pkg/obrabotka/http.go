package obrabotka

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"github.com/tym83/assnetes/pkg/bsl"
)

// HTTP of the platform: HTTPСоединение, HTTPЗапрос, HTTPОтвет. Unlike
// OneScript, certificates of servers are checked, by the system's roots.

type httpConnection struct {
	base    string
	user    string
	pass    string
	timeout time.Duration
}

func (*httpConnection) TypeName() string { return "HTTPСоединение" }

type httpRequest struct {
	path    string
	headers *bsl.Map
	body    []byte
}

func (*httpRequest) TypeName() string { return "HTTPЗапрос" }

func (r *httpRequest) Get(name string) (bsl.Value, bool) {
	switch strings.ToLower(name) {
	case "адресресурса", "resourceaddress":
		return r.path, true
	case "заголовки", "headers":
		return r.headers, true
	}
	return nil, false
}

func (r *httpRequest) Set(name string, v bsl.Value) error {
	switch strings.ToLower(name) {
	case "адресресурса", "resourceaddress":
		r.path = bsl.ToString(v)
		return nil
	}
	return fmt.Errorf("Поле объекта не обнаружено (%s)", name)
}

func (r *httpRequest) CallMethod(_ *bsl.Interp, name string, args []*bsl.Ref) (bsl.Value, error) {
	switch strings.ToLower(name) {
	case "установитьтелоизстроки", "setbodyfromstring":
		r.body = []byte(bsl.ToString(bsl.Arg(args, 0)))
		return nil, nil
	case "получитьтелокакстроку", "getbodyasstring":
		return string(r.body), nil
	}
	return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}

type httpResponse struct {
	code    int
	headers *bsl.Map
	body    []byte
	ctype   string
}

func (*httpResponse) TypeName() string { return "HTTPОтвет" }

func (r *httpResponse) Get(name string) (bsl.Value, bool) {
	switch strings.ToLower(name) {
	case "кодсостояния", "statuscode":
		return bsl.NewInt(int64(r.code)), true
	case "заголовки", "headers":
		return r.headers, true
	}
	return nil, false
}

func (r *httpResponse) CallMethod(_ *bsl.Interp, name string, _ []*bsl.Ref) (bsl.Value, error) {
	switch strings.ToLower(name) {
	case "получитьтелокакстроку", "getbodyasstring":
		rd, err := charset.NewReader(bytes.NewReader(r.body), r.ctype)
		if err != nil {
			return string(r.body), nil
		}
		b, err := io.ReadAll(rd)
		if err != nil {
			return string(r.body), nil
		}
		return string(b), nil
	}
	return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}

var httpMethods = map[string]string{
	"получить": "GET", "get": "GET",
	"отправитьдляобработки": "POST", "post": "POST",
	"записать": "PUT", "put": "PUT",
	"удалить": "DELETE", "delete": "DELETE",
	"изменить": "PATCH", "patch": "PATCH",
	"получитьзаголовки": "HEAD", "head": "HEAD",
}

func (c *httpConnection) CallMethod(it *bsl.Interp, name string, args []*bsl.Ref) (bsl.Value, error) {
	method, ok := httpMethods[strings.ToLower(name)]
	reqArg := 0
	if strings.EqualFold(name, "ВызватьHTTPМетод") || strings.EqualFold(name, "CallHTTPMethod") {
		method, ok, reqArg = strings.ToUpper(bsl.ToString(bsl.Arg(args, 0))), true, 1
	}
	if !ok {
		return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
	}
	req, isReq := bsl.Arg(args, reqArg).(*httpRequest)
	if !isReq {
		return nil, fmt.Errorf("ожидается HTTPЗапрос")
	}
	ctx, cancel := context.WithTimeout(itContext(it), c.timeout)
	defer cancel()
	hr, err := http.NewRequestWithContext(ctx, method, c.base+req.path, bytes.NewReader(req.body))
	if err != nil {
		return nil, err
	}
	if req.headers != nil {
		for _, k := range req.headers.Keys() {
			v, _ := req.headers.Lookup(k)
			hr.Header.Set(bsl.ToString(k), bsl.ToString(v))
		}
	}
	if hr.Header.Get("User-Agent") == "" {
		// some servers, the Bank of Russia among them, turn away Go's default
		hr.Header.Set("User-Agent", "assnetes-obrabotchik/1 (1Zh:Orkestrator 8)")
	}
	if c.user != "" {
		hr.SetBasicAuth(c.user, c.pass)
	}
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		return nil, fmt.Errorf("Ошибка работы с Интернет: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	h := bsl.NewMap()
	for k := range resp.Header {
		h.Insert(k, resp.Header.Get(k))
	}
	return &httpResponse{code: resp.StatusCode, headers: h, body: body, ctype: resp.Header.Get("Content-Type")}, nil
}

// itContext is the context the interpreter runs under.
func itContext(it *bsl.Interp) context.Context {
	if c := it.Context(); c != nil {
		return c
	}
	return context.Background()
}

func registerHTTP(it *bsl.Interp) {
	it.DefineType(func(_ *bsl.Interp, args []bsl.Value) (bsl.Value, error) {
		host := ""
		if len(args) > 0 {
			host = bsl.ToString(args[0])
		}
		c := &httpConnection{timeout: 60 * time.Second}
		secure := len(args) > 6 && args[6] != nil && bsl.TypeOf(args[6]) != "Неопределено"
		switch {
		case strings.HasPrefix(host, "http://"), strings.HasPrefix(host, "https://"):
			c.base = strings.TrimSuffix(host, "/")
		case secure:
			c.base = "https://" + host
		default:
			c.base = "http://" + host
		}
		if len(args) > 1 {
			if n, ok := args[1].(bsl.Number); ok && n.Sign() > 0 {
				c.base += fmt.Sprintf(":%d", n.Int64())
			}
		}
		if len(args) > 2 {
			c.user = bsl.ToString(args[2])
		}
		if len(args) > 3 {
			c.pass = bsl.ToString(args[3])
		}
		if len(args) > 5 {
			if n, ok := args[5].(bsl.Number); ok && n.Sign() > 0 {
				c.timeout = time.Duration(n.Int64()) * time.Second
			}
		}
		return c, nil
	}, "HTTPСоединение", "HTTPConnection")
	it.DefineType(func(_ *bsl.Interp, args []bsl.Value) (bsl.Value, error) {
		r := &httpRequest{headers: bsl.NewMap()}
		if len(args) > 0 {
			r.path = bsl.ToString(args[0])
		}
		if len(args) > 1 {
			if m, ok := args[1].(*bsl.Map); ok {
				r.headers = m
			}
		}
		return r, nil
	}, "HTTPЗапрос", "HTTPRequest")
	it.DefineType(func(_ *bsl.Interp, _ []bsl.Value) (bsl.Value, error) {
		return secureConn{}, nil
	}, "ЗащищенноеСоединениеOpenSSL", "OpenSSLSecureConnection")
}

type secureConn struct{}

func (secureConn) TypeName() string { return "ЗащищенноеСоединениеOpenSSL" }

// ЧтениеXML: the pull reader of the platform, the part modules need.

type xmlReader struct {
	dec   *xml.Decoder
	kind  string
	name  string
	value string
	attrs []xml.Attr
}

func (*xmlReader) TypeName() string { return "ЧтениеXML" }

func (r *xmlReader) Get(name string) (bsl.Value, bool) {
	switch strings.ToLower(name) {
	case "типузла", "nodetype":
		return xmlNodeType(r.kind), true
	case "имя", "localname", "name":
		return r.name, true
	case "значение", "value":
		return r.value, true
	}
	return nil, false
}

type xmlNodeType string

func (xmlNodeType) TypeName() string { return "ТипУзлаXML" }
func (x xmlNodeType) EqualTo(v bsl.Value) bool {
	y, ok := v.(xmlNodeType)
	return ok && x == y
}

func (r *xmlReader) CallMethod(_ *bsl.Interp, name string, args []*bsl.Ref) (bsl.Value, error) {
	switch strings.ToLower(name) {
	case "установитьстроку", "settostring":
		r.dec = xml.NewDecoder(strings.NewReader(bsl.ToString(bsl.Arg(args, 0))))
		r.dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
		return nil, nil
	case "прочитать", "read":
		if r.dec == nil {
			return false, nil
		}
		for {
			tok, err := r.dec.Token()
			if err == io.EOF {
				return false, nil
			}
			if err != nil {
				return nil, fmt.Errorf("Ошибка разбора XML: %v", err)
			}
			switch t := tok.(type) {
			case xml.StartElement:
				r.kind, r.name, r.value, r.attrs = "НачалоЭлемента", t.Name.Local, "", t.Attr
				return true, nil
			case xml.EndElement:
				r.kind, r.name, r.value, r.attrs = "КонецЭлемента", t.Name.Local, "", nil
				return true, nil
			case xml.CharData:
				if strings.TrimSpace(string(t)) == "" {
					continue
				}
				r.kind, r.name, r.value = "Текст", "", string(t)
				return true, nil
			}
		}
	case "значениеатрибута", "attributevalue", "getattribute":
		n := bsl.ToString(bsl.Arg(args, 0))
		for _, a := range r.attrs {
			if a.Name.Local == n {
				return a.Value, nil
			}
		}
		return bsl.Undefined{}, nil
	case "закрыть", "close":
		r.dec = nil
		return nil, nil
	}
	return nil, fmt.Errorf("Метод объекта не обнаружен (%s)", name)
}

type xmlNodeTypes struct{}

func (xmlNodeTypes) TypeName() string { return "ТипУзлаXML" }
func (xmlNodeTypes) Get(name string) (bsl.Value, bool) {
	switch strings.ToLower(name) {
	case "началоэлемента", "startelement":
		return xmlNodeType("НачалоЭлемента"), true
	case "конецэлемента", "endelement":
		return xmlNodeType("КонецЭлемента"), true
	case "текст", "text":
		return xmlNodeType("Текст"), true
	}
	return nil, false
}

func registerXML(it *bsl.Interp) {
	it.SetGlobal(xmlNodeTypes{}, "ТипУзлаXML", "XMLNodeType")
	it.DefineType(func(_ *bsl.Interp, _ []bsl.Value) (bsl.Value, error) { return &xmlReader{}, nil }, "ЧтениеXML", "XMLReader")
}
