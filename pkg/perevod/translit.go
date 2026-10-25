package perevod

import (
	"strings"

	"golang.org/x/net/idna"
)

var translitTable = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo", 'ж': "zh",
	'з': "z", 'и': "i", 'й': "j", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o",
	'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "h", 'ц': "c",
	'ч': "ch", 'ш': "sh", 'щ': "shh", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu",
	'я': "ya",
	// Kazakh, Ukrainian, Belarusian letters
	'і': "i", 'ї': "yi", 'є': "ye", 'ґ': "g", 'ў': "u", 'ә': "a", 'ғ': "g", 'қ': "q",
	'ң': "n", 'ө': "o", 'ұ': "u", 'ү': "u", 'һ': "h",
}

// Translit turns a name into a DNS label the API accepts: lower case
// Latin letters, digits and dashes, at most 63 characters. The same name
// always gives the same result, so a name can be looked up by its
// Cyrillic spelling.
func Translit(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		var part string
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			part = string(r)
		default:
			if t, ok := translitTable[r]; ok {
				part = t
			}
		}
		if part == "" {
			if _, letter := translitTable[r]; letter {
				continue // ъ and ь vanish without breaking the word
			}
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
			continue
		}
		b.WriteString(part)
		dash = false
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 63 {
		out = strings.TrimRight(out[:63], "-")
	}
	if out == "" {
		out = "bez-imeni"
	}
	return out
}

// IsAPIName reports whether s already is a name the API accepts as it is.
func IsAPIName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

// Name returns the API name for a name as written.
func Name(s string) string {
	if IsAPIName(s) {
		return s
	}
	return Translit(s)
}

func punyEncode(group string) string {
	a, err := idna.Lookup.ToASCII(group)
	if err != nil {
		return group
	}
	return a
}

func punyDecode(group string) string {
	u, err := idna.Lookup.ToUnicode(group)
	if err != nil {
		return group
	}
	return u
}

// AnnotationKey turns «оркестратор.бух/наименование» into a key the API
// accepts: the prefix in punycode, the name transliterated.
func AnnotationKey(k string) string {
	prefix, name, ok := strings.Cut(k, "/")
	if !ok {
		return labelName(k)
	}
	return punyEncode(prefix) + "/" + labelName(name)
}

// annotationNames are names of the distribution's own annotations, so they
// read back in Russian, not in transliteration.
var annotationNames = []string{"наименование", "метки", "имена", "пометка-удаления", "служебная-записка",
	"дата-запрета", "главбух", "курс", "стоимость"}

// AnnotationKeyRu turns an annotation or label key back into Cyrillic.
func AnnotationKeyRu(k string) string {
	prefix, name, ok := strings.Cut(k, "/")
	if !ok {
		return k
	}
	if prefix == Group {
		for _, n := range annotationNames {
			if Translit(n) == name {
				name = n
				break
			}
		}
	}
	return punyDecode(prefix) + "/" + name
}

// labelName keeps a valid label name and transliterates anything else.
func labelName(s string) string {
	ok := s != "" && len(s) <= 63
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			ok = false
		}
	}
	if ok {
		return s
	}
	return Translit(s)
}

// LabelValue turns a label value into one the API accepts.
func LabelValue(s string) string {
	if s == "" {
		return s
	}
	return labelName(s)
}
