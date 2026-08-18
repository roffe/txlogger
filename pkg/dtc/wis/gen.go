//go:build ignore

// gen.go builds wis.zip from the raw SAAB WIS dumps in ../9400, ../9440 and
// ../9600 (not committed to git, ~1.7GB). It extracts only the DTC fault
// diagnosis documents and the SVG images they reference:
//
//	index.json           model -> year -> []Entry (see wis.go)
//	<model>/doc*.htm     cleaned fault diagnosis documents
//	<model>/img/<id>.svg wiring diagrams referenced by the documents
//
// Run with: go generate ./pkg/dtc/wis
package main

import (
	"archive/zip"
	"compress/flate"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

type entry struct {
	Code    string `json:"c"`
	System  string `json:"s"`
	Title   string `json:"t,omitempty"`
	Symptom string `json:"y,omitempty"`
	Doc     string `json:"d"`
}

var (
	reScript = regexp.MustCompile(`(?is)<\s*script\b[^>]*>.*?</script\b[^>]*>`)
	reImg    = regexp.MustCompile(`(?is)<\s*img\b[^>]*>`)
	reWisimg = regexp.MustCompile(`(?is)<\s*a\b[^>]*href="wisimg://i([0-9a-zA-Z]+)"[^>]*>.*?</a\b[^>]*>`)
	reAnchor = regexp.MustCompile(`(?is)<\s*a\b[^>]*>.*?</a\b[^>]*>`)
	reHsides = regexp.MustCompile(`(?is)<table frame='hsides\b[^>]*>.*?</table>`)
	reH2     = regexp.MustCompile(`(?is)<h2[^>]*>(.*?)</h2>`)
	reTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	rePlaceh = regexp.MustCompile(`<wisimg id="([0-9a-zA-Z]+)"/>`)
)

// sit num="0602" is "Fault diagnosis, diagnostic trouble codes" in the WIS
// section tree used by the 9400 and 9600 dumps.
type modelyearXML struct {
	Scts []struct {
		Scs []struct {
			Name string `xml:"name"`
			Sits []struct {
				Num  string `xml:"num,attr"`
				Sies []struct {
					DocID string   `xml:"docid,attr"`
					Names []string `xml:"name"`
					Diag  struct {
						Sympdesc string `xml:"sympdesc"`
					} `xml:"diagnostic"`
				} `xml:"sie"`
			} `xml:"sit"`
		} `xml:"sc"`
	} `xml:"sct"`
}

type ecmlistXML struct {
	Ecms []struct {
		Name string `xml:"name,attr"`
		Dtcs []struct {
			Fcode string `xml:"fcode"`
			Cms   []struct {
				DocID    string `xml:"docid,attr"`
				Sympdesc string `xml:"diagnostic>sympdesc"`
			} `xml:"cm"`
		} `xml:"dtc"`
	} `xml:"ecm"`
}

func parseXML(fn string, v any) error {
	f, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) {
		return charmap.ISO8859_1.NewDecoder().Reader(in), nil
	}
	return dec.Decode(v)
}

// zipDir lazily opens the per-thousand gbN.zip doc archives of one model.
type zipDir struct {
	dir   string
	zips  map[string]*zip.ReadCloser
	files map[string]*zip.File // lowercase name -> file, per archive merged
}

func newZipDir(dir string) *zipDir {
	return &zipDir{dir: dir, zips: map[string]*zip.ReadCloser{}, files: map[string]*zip.File{}}
}

func (z *zipDir) read(archive, name string) ([]byte, error) {
	if _, ok := z.zips[archive]; !ok {
		rc, err := zip.OpenReader(filepath.Join(z.dir, archive))
		if err != nil {
			return nil, err
		}
		z.zips[archive] = rc
		for _, f := range rc.File {
			z.files[archive+"#"+strings.ToLower(f.Name)] = f
		}
	}
	f, ok := z.files[archive+"#"+strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("%s: no %s", archive, name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func win1252(b []byte) string {
	s, _ := charmap.Windows1252.NewDecoder().Bytes(b)
	return string(s)
}

// clean applies the same document scrubbing the original PHP page did:
// scripts, gif icons, the "click the tab" intro table and dead links go;
// wisimg image links become <wisimg id=".."/> placeholders that Render
// swaps for inline SVG.
func clean(c []byte) []byte {
	c = reScript.ReplaceAll(c, nil)
	c = reImg.ReplaceAll(c, nil)
	c = reWisimg.ReplaceAll(c, []byte(`<wisimg id="$1"/>`))
	c = reAnchor.ReplaceAll(c, nil)
	if loc := reHsides.FindIndex(c); loc != nil {
		c = append(c[:loc[0]:loc[0]], c[loc[1]:]...)
	}
	return c
}

func title(c []byte) string {
	m := reH2.FindSubmatch(c)
	if m == nil {
		return ""
	}
	t := reTag.ReplaceAll(m[1], nil)
	s := strings.TrimSpace(strings.ReplaceAll(win1252(t), "&nbsp;", " "))
	return strings.ReplaceAll(s, "&amp;", "&")
}

func main() {
	src := ".."
	if len(os.Args) > 1 {
		src = os.Args[1]
	}

	out, err := os.Create("wis.zip")
	if err != nil {
		log.Fatal(err)
	}
	zw := zip.NewWriter(out)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestCompression)
	})

	index := map[string]map[string][]entry{} // model -> year -> entries
	reYear := regexp.MustCompile(`(\d{4})(ECMGB|gb)\.xml$`)

	for _, model := range []string{"9400", "9440", "9600"} {
		dir := filepath.Join(src, model)
		docs := newZipDir(dir)
		index[model] = map[string][]entry{}
		titles := map[string]string{} // doc path -> extracted heading
		images := map[string]bool{}   // svg ids referenced by written docs
		written := map[string]bool{}  // doc paths already in the zip

		// writeDoc extracts, cleans and stores one document, returning its
		// path in the archive and its heading.
		writeDoc := func(docid string) (string, string, error) {
			var name string
			id := strings.ToLower(docid)
			if n, ok := strings.CutPrefix(id, "ida"); ok {
				name = "docida" + n + ".htm"
				id = n
			} else {
				name = "doc" + id + ".htm"
			}
			num, err := strconv.Atoi(id)
			if err != nil {
				return "", "", fmt.Errorf("bad docid %q", docid)
			}
			p := model + "/" + name
			if written[p] {
				return p, titles[p], nil
			}
			c, err := docs.read(fmt.Sprintf("gb%d.zip", num/1000), name)
			if err != nil {
				return "", "", err
			}
			c = clean(c)
			for _, m := range rePlaceh.FindAllSubmatch(c, -1) {
				images[string(m[1])] = true
			}
			w, err := zw.Create(p)
			if err != nil {
				return "", "", err
			}
			if _, err := w.Write(c); err != nil {
				return "", "", err
			}
			written[p] = true
			titles[p] = title(c)
			return p, titles[p], nil
		}

		xmls, _ := filepath.Glob(filepath.Join(dir, "*.xml"))
		for _, fn := range xmls {
			m := reYear.FindStringSubmatch(fn)
			if m == nil {
				continue
			}
			year := m[1]
			var entries []entry
			if m[2] == "ECMGB" {
				var x ecmlistXML
				if err := parseXML(fn, &x); err != nil {
					log.Fatalf("%s: %v", fn, err)
				}
				for _, ecm := range x.Ecms {
					for _, d := range ecm.Dtcs {
						for _, cm := range d.Cms {
							p, t, err := writeDoc(cm.DocID)
							if err != nil {
								log.Fatalf("%s: %v", fn, err)
							}
							entries = append(entries, entry{
								Code:    strings.ToUpper(strings.TrimSpace(d.Fcode)),
								System:  ecm.Name,
								Title:   t,
								Symptom: strings.TrimSpace(cm.Sympdesc),
								Doc:     p,
							})
						}
					}
				}
			} else {
				var x modelyearXML
				if err := parseXML(fn, &x); err != nil {
					log.Fatalf("%s: %v", fn, err)
				}
				for _, sct := range x.Scts {
					for _, sc := range sct.Scs {
						for _, sit := range sc.Sits {
							if sit.Num != "0602" {
								continue
							}
							for _, sie := range sit.Sies {
								p, t, err := writeDoc(sie.DocID)
								if err != nil {
									log.Fatalf("%s: %v", fn, err)
								}
								entries = append(entries, entry{
									Code:    strings.ToUpper(strings.TrimSpace(strings.Join(sie.Names, "/"))),
									System:  strings.TrimSpace(sc.Name),
									Title:   t,
									Symptom: strings.TrimSpace(sie.Diag.Sympdesc),
									Doc:     p,
								})
							}
						}
					}
				}
			}
			if len(entries) == 0 {
				log.Printf("warning: %s: no DTC entries", fn)
				continue
			}
			index[model][year] = entries
		}

		// pull the referenced wiring diagrams out of images.zip
		imgs := newZipDir(dir)
		var missing int
		ids := make([]string, 0, len(images))
		for id := range images {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			c, err := imgs.read("images.zip", "images/"+id+".svg")
			if err != nil {
				missing++
				continue
			}
			w, err := zw.Create(model + "/img/" + id + ".svg")
			if err != nil {
				log.Fatal(err)
			}
			if _, err := w.Write(c); err != nil {
				log.Fatal(err)
			}
		}
		log.Printf("%s: %d years, %d docs, %d images (%d missing)",
			model, len(index[model]), len(written), len(images)-missing, missing)
	}

	iw, err := zw.Create("index.json")
	if err != nil {
		log.Fatal(err)
	}
	enc := json.NewEncoder(iw)
	if err := enc.Encode(index); err != nil {
		log.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}
	if err := out.Close(); err != nil {
		log.Fatal(err)
	}
	st, _ := os.Stat("wis.zip")
	log.Printf("wis.zip: %d MB", st.Size()>>20)
}
