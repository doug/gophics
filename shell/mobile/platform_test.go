package mobile_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/mobile"
)

// Each capability is nil until a host wires it, and that is the contract an app
// depends on: ctx.Share() == nil is how a widget knows to hide the button. A
// capability that returned a working-looking value with no backend would be
// undetectable, which is the one failure mode the shell design rules out.
func TestCapabilitiesAreNilWithoutAHost(t *testing.T) {
	b := mobile.NewBridge(nil)
	cases := map[string]bool{
		"Share":         b.Share() == nil,
		"Notifier":      b.Notifier() == nil,
		"SecureStorage": b.SecureStorage() == nil,
		"FilePicker":    b.FilePicker() == nil,
		"Geolocation":   b.Geolocation() == nil,
		"Preferences":   b.Preferences() == nil,
		"Connectivity":  b.Connectivity() == nil,
		"Battery":       b.Battery() == nil,
	}
	for name, isNil := range cases {
		if !isNil {
			t.Errorf("%s must be nil before a host registers a backend", name)
		}
	}
}

type fakePlatform struct {
	b        *mobile.Bridge
	shared   shell.ShareItem
	notified shell.Notification
	secrets  map[string]string
	failSave bool
}

func (f *fakePlatform) Share(reqID int, title, text, url, fileName string, data []byte) {
	f.shared = shell.ShareItem{Title: title, Text: text, URL: url, FileName: fileName, FileData: data}
	f.b.DeliverShareResult(reqID, "")
}
func (f *fakePlatform) AuthorizeNotify(reqID int) { f.b.DeliverNotifyPermission(reqID, true) }
func (f *fakePlatform) Notify(title, body, tag string) {
	f.notified = shell.Notification{Title: title, Body: body, Tag: tag}
}
func (f *fakePlatform) SecureGet(key string) string { return f.secrets[key] }
func (f *fakePlatform) SecureHas(key string) bool   { _, ok := f.secrets[key]; return ok }
func (f *fakePlatform) SecureSet(key, value string) string {
	f.secrets[key] = value
	return ""
}
func (f *fakePlatform) SecureDelete(key string) string { delete(f.secrets, key); return "" }
func (f *fakePlatform) PickFiles(reqID int, accept string, multiple bool) {
	f.b.DeliverPickedFile(reqID, "a.txt", []byte("one"))
	if multiple {
		f.b.DeliverPickedFile(reqID, "b.txt", []byte("two"))
	}
	f.b.DeliverPickedDone(reqID)
}
func (f *fakePlatform) SaveFile(reqID int, name, accept string, data []byte) {
	if f.failSave {
		f.b.DeliverSaveDone(reqID, "disk full")
		return
	}
	f.b.DeliverSaveDone(reqID, "")
}

func TestShareAndNotifyRoundTrip(t *testing.T) {
	b := mobile.NewBridge(nil)
	h := &fakePlatform{b: b}
	b.SetShareHost(h)
	b.SetNotifyHost(h)

	var shareErr error
	called := false
	b.Share().Share(shell.ShareItem{Title: "t", Text: "hello", URL: "https://x"}, func(err error) {
		called, shareErr = true, err
	})
	if !called || shareErr != nil {
		t.Fatalf("share callback: called=%v err=%v", called, shareErr)
	}
	if h.shared.Text != "hello" || h.shared.URL != "https://x" {
		t.Errorf("host received %+v", h.shared)
	}

	var perm shell.Permission
	b.Notifier().Authorize(func(p shell.Permission) { perm = p })
	if perm != shell.PermissionGranted {
		t.Errorf("permission = %v, want granted", perm)
	}
	b.Notifier().Notify(shell.Notification{Title: "Done", Body: "b", Tag: "x"})
	if h.notified.Title != "Done" {
		t.Errorf("notification = %+v", h.notified)
	}
}

// SecureStorage is the one synchronous capability here, so the interesting case
// is the one a cache cannot serve: a stored empty string is present, and must
// not read as absent.
func TestSecureStorageDistinguishesEmptyFromAbsent(t *testing.T) {
	b := mobile.NewBridge(nil)
	h := &fakePlatform{b: b, secrets: map[string]string{}}
	b.SetSecureHost(h)
	s := b.SecureStorage()

	if _, ok := s.Get("token"); ok {
		t.Error("absent key reported as present")
	}
	if err := s.Set("token", ""); err != nil {
		t.Fatal(err)
	}
	v, ok := s.Get("token")
	if !ok || v != "" {
		t.Errorf("Get after storing empty = (%q, %v), want (\"\", true)", v, ok)
	}
	if err := s.Delete("token"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("token"); ok {
		t.Error("key survived Delete")
	}
}

func TestFilePickerCollectsEveryDeliveredFile(t *testing.T) {
	b := mobile.NewBridge(nil)
	h := &fakePlatform{b: b}
	b.SetFileHost(h)
	fp := b.FilePicker()

	var got []shell.PickedFile
	fp.Open(shell.OpenOptions{Multiple: true, Accept: []string{".txt", "text/plain"}}, func(f []shell.PickedFile, err error) {
		if err != nil {
			t.Fatal(err)
		}
		got = f
	})
	if len(got) != 2 || got[0].Name != "a.txt" || string(got[1].Data) != "two" {
		t.Fatalf("picked %+v", got)
	}

	h.failSave = true
	var saveErr error
	fp.Save(shell.SaveOptions{Name: "out.txt"}, []byte("x"), func(err error) { saveErr = err })
	if saveErr == nil {
		t.Error("a failing save must report the error")
	}
}

// The host owns the buffer it hands across the bind boundary and may reuse it,
// so the bridge has to copy. Aliasing here would corrupt a multi-select where
// the host reads each file into one scratch buffer.
func TestPickedFileDataIsCopied(t *testing.T) {
	b := mobile.NewBridge(nil)
	var got []shell.PickedFile
	b.SetFileHost(reuseHost{b: b})
	b.FilePicker().Open(shell.OpenOptions{}, func(f []shell.PickedFile, err error) { got = f })
	if len(got) != 2 {
		t.Fatalf("got %d files", len(got))
	}
	if string(got[0].Data) == string(got[1].Data) {
		t.Errorf("both files hold %q — the bridge aliased the host's buffer", got[0].Data)
	}
}

type reuseHost struct{ b *mobile.Bridge }

func (h reuseHost) PickFiles(reqID int, accept string, multiple bool) {
	scratch := []byte("one")
	h.b.DeliverPickedFile(reqID, "a", scratch)
	copy(scratch, "two") // the host reuses its buffer for the next file
	h.b.DeliverPickedFile(reqID, "b", scratch)
	h.b.DeliverPickedDone(reqID)
}
func (h reuseHost) SaveFile(reqID int, name, accept string, data []byte) {}

// Connectivity and Battery start unknown rather than optimistic: a phone that
// has not been told is not "online", and reporting it that way makes an app
// skip its offline path exactly at launch.
func TestPushedStateStartsUnknown(t *testing.T) {
	b := mobile.NewBridge(nil)
	if b.Connectivity() != nil {
		t.Error("Connectivity must be nil until the host reports reachability")
	}
	seen := []bool{}
	b.SetOnline(true)
	c := b.Connectivity()
	if c == nil {
		t.Fatal("Connectivity must appear once the host reports")
	}
	c.OnChange(func(online bool) { seen = append(seen, online) })
	if !c.Online() {
		t.Error("Online() disagrees with what the host reported")
	}
	b.SetOnline(true) // unchanged: must not fire
	b.SetOnline(false)
	if len(seen) != 1 || seen[0] {
		t.Errorf("subscriber saw %v, want exactly one false", seen)
	}
}

// The launch URL is "initial" only before the app has drawn anything. A link
// arriving later is a running-app deep link, and reporting it as Initial would
// make a cold-start branch run mid-session.
func TestInitialLinkOnlyBeforeTheFirstFrame(t *testing.T) {
	h, err := app.NewHandler(fieldApp{}, app.Config{Font: goregular.TTF})
	if err != nil {
		t.Fatal(err)
	}
	b := mobile.NewBridge(h)
	b.DeliverLink("myapp://launch")
	if got := b.Links().Initial(); got != "myapp://launch" {
		t.Errorf("Initial() = %q before any frame", got)
	}
	b.Resize(100, 100, 1)
	b.Snapshot(0.016)

	var live []string
	b.Links().OnLink(func(u string) { live = append(live, u) })
	b.DeliverLink("myapp://later")

	if got := b.Links().Initial(); got != "myapp://launch" {
		t.Errorf("Initial() = %q after a later link, want the launch URL", got)
	}
	if len(live) != 1 || live[0] != "myapp://later" {
		t.Errorf("OnLink saw %v", live)
	}
}
