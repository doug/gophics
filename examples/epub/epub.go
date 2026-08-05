package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
)

// Book is a parsed EPUB: metadata plus ordered chapters of text blocks.
type Book struct {
	Title    string
	Author   string
	Chapters []Chapter
}

// Chapter is one spine document, reduced to its title + body blocks.
type Chapter struct {
	Title  string
	Blocks []Block
}

// Block is a paragraph or a heading of already-flattened text.
type Block struct {
	Heading bool
	Text    string
}

// --- the bundled sample book (authored here, so it's self-contained and has no
// copyright entanglements). Each chapter is real XHTML that the parser below
// extracts, so the demo exercises the whole epub pipeline. ---

type srcChapter struct{ file, xhtml string }

var sampleMeta = struct{ title, author string }{"The Lantern-Keeper's Almanac", "A. Marlowe"}

var sampleChapters = []srcChapter{
	{"ch1.xhtml", `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>I</title></head><body>
<h2>I. The Tide-Clock</h2>
<p>The lighthouse kept two kinds of time. One was the plain time of the mainland,
which arrived each morning folded inside the mail-boat and was, by evening,
already wrong. The other was the tide-clock — a brass dial with no numbers,
only a slow hand that leaned toward the sea and away from it, patient as breath.</p>
<p>Elka had tended both since she was nine, when the old keeper handed her the
oil-can and said nothing at all, which she later understood to be the whole of
the instruction. You learned the light by living inside its turning.</p>
<p>On clear nights the beam swept the water like a long white thought, and the
gulls, caught in it, flared briefly into paper and were gone.</p>
</body></html>`},
	{"ch2.xhtml", `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>II</title></head><body>
<h2>II. What the Almanac Said</h2>
<p>The almanac was not printed. It was kept — added to, argued with, occasionally
crossed out — by every keeper who had ever climbed the stair. Its pages
predicted nothing and remembered everything: the winter the herring failed, the
night three lamps were lit at once and no one afterward would say why.</p>
<p>Elka read it the way other people read weather: not for facts but for warning.
Near the back, in a hand browner than the rest, someone had written a single
line and underlined it twice. <em>When the sea keeps time, do not correct it.</em></p>
<p>She had corrected it once. She did not intend to make that mistake again.</p>
</body></html>`},
	{"ch3.xhtml", `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>III</title></head><body>
<h2>III. The Visitor Below the Waterline</h2>
<p>It came the way fog comes: not arriving so much as already having been there.
A knock, if a knock can be felt through stone and forty feet of cold water — one,
then two, then the long patient silence of something willing to wait.</p>
<p>Elka set down the oil-can. Above her the light turned, indifferent and exact,
throwing its slow white sentence out across the dark. Below her the tide-clock
leaned, and leaned, and did not come back.</p>
<p>She opened the almanac to a clean page, dipped the pen, and began — as every
keeper before her had begun — to write down precisely what she saw, so that the
next one to climb the stair would know it was not the first time.</p>
</body></html>`},
}

// loadBook builds the sample epub in memory and parses it back — the whole
// pipeline (zip → container.xml → OPF spine → XHTML). It panics on failure
// because the input is generated here and must always be valid.
func loadBook() *Book {
	data := buildEPUB()
	b, err := parseEPUB(data)
	if err != nil {
		panic("epub: " + err.Error())
	}
	return b
}

// buildEPUB assembles a spec-shaped .epub (stored mimetype first, META-INF
// container, OPF package, XHTML chapters) as an in-memory zip.
func buildEPUB() []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// The mimetype entry must be first and uncompressed (EPUB OCF rule).
	mw, _ := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	_, _ = io.WriteString(mw, "application/epub+zip")

	write := func(name, body string) {
		w, _ := zw.Create(name)
		_, _ = io.WriteString(w, body)
	}

	write("META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)

	var manifest, spine strings.Builder
	for i, c := range sampleChapters {
		id := fmt.Sprintf("ch%d", i+1)
		fmt.Fprintf(&manifest, `    <item id="%s" href="%s" media-type="application/xhtml+xml"/>`+"\n", id, c.file)
		fmt.Fprintf(&spine, `    <itemref idref="%s"/>`+"\n", id)
		write("OEBPS/"+c.file, c.xhtml)
	}
	write("OEBPS/content.opf", fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>%s</dc:title>
    <dc:creator>%s</dc:creator>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
%s  </manifest>
  <spine>
%s  </spine>
</package>`, sampleMeta.title, sampleMeta.author, manifest.String(), spine.String()))

	_ = zw.Close()
	return buf.Bytes()
}

// parseEPUB reads an .epub: locate the OPF via META-INF/container.xml, read its
// metadata + spine order, then extract each spine document's text blocks.
func parseEPUB(data []byte) (*Book, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}

	// container.xml has the OPF path in the rootfile's full-path attribute.
	var cont struct {
		Rootfile struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := unmarshalZip(files, "META-INF/container.xml", &cont); err != nil {
		return nil, err
	}
	opfPath := cont.Rootfile.FullPath
	if opfPath == "" {
		return nil, fmt.Errorf("no rootfile in container.xml")
	}

	var pkg struct {
		Title   string `xml:"metadata>title"`
		Creator string `xml:"metadata>creator"`
		Items   []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
		} `xml:"manifest>item"`
		Spine []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"spine>itemref"`
	}
	if err := unmarshalZip(files, opfPath, &pkg); err != nil {
		return nil, err
	}

	href := map[string]string{}
	for _, it := range pkg.Items {
		href[it.ID] = it.Href
	}
	base := path.Dir(opfPath)

	book := &Book{Title: pkg.Title, Author: pkg.Creator}
	for _, ref := range pkg.Spine {
		rel, ok := href[ref.IDRef]
		if !ok {
			continue
		}
		raw, err := readZip(files, path.Join(base, rel))
		if err != nil {
			return nil, err
		}
		blocks := extractBlocks(raw)
		ch := Chapter{Blocks: blocks}
		if len(blocks) > 0 && blocks[0].Heading {
			ch.Title = blocks[0].Text
		} else {
			ch.Title = fmt.Sprintf("Chapter %d", len(book.Chapters)+1)
		}
		book.Chapters = append(book.Chapters, ch)
	}
	return book, nil
}

// extractBlocks walks an XHTML document and reduces it to heading/paragraph
// blocks of flattened, whitespace-collapsed text. Inline markup (em/strong) is
// kept as plain text.
func extractBlocks(xhtml []byte) []Block {
	dec := xml.NewDecoder(bytes.NewReader(xhtml))
	var blocks []Block
	var cur *Block
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch strings.ToLower(t.Name.Local) {
			case "h1", "h2", "h3", "h4":
				cur = &Block{Heading: true}
			case "p":
				cur = &Block{}
			}
		case xml.CharData:
			if cur != nil {
				cur.Text += string(t)
			}
		case xml.EndElement:
			switch strings.ToLower(t.Name.Local) {
			case "h1", "h2", "h3", "h4", "p":
				if cur != nil {
					if s := strings.Join(strings.Fields(cur.Text), " "); s != "" {
						cur.Text = s
						blocks = append(blocks, *cur)
					}
					cur = nil
				}
			}
		}
	}
	return blocks
}

func readZip(files map[string]*zip.File, name string) ([]byte, error) {
	f, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("epub: missing %q", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func unmarshalZip(files map[string]*zip.File, name string, v any) error {
	raw, err := readZip(files, name)
	if err != nil {
		return err
	}
	return xml.Unmarshal(raw, v)
}
