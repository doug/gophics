//go:build linux

// D-Bus serving: replies and the type writers a server needs.
//
// dbus_linux.go is client-shaped — send a call, wait for the matching reply,
// discard everything else. AT-SPI inverts that: the screen reader calls *us*,
// on its own schedule, and we answer. This file adds the missing half.
//
// Nothing here changes the wire format; it reuses the same msgBuf, alignment
// arithmetic and header-field writer. What it adds is the message kinds a
// server sends (METHOD_RETURN, ERROR) and the value writers AT-SPI's
// signatures need — object paths, int32, structs, dictionaries and typed
// variants — none of which the file portal happened to require.

package platform

import (
	"encoding/binary"
	"math"
)

// --- extra msgBuf writers ---------------------------------------------------

// i32 writes a signed 32-bit integer ("i"). Alignment and width match u32; the
// distinction is in the signature, which the decoder on the far side uses to
// interpret the bits.
func (b *msgBuf) i32(v int32) { b.u32(uint32(v)) }

// i16 writes a signed 16-bit integer ("n"), used by Component.GetMDIZOrder.
func (b *msgBuf) i16(v int16) {
	b.padTo(2)
	b.data = append(b.data, 0, 0)
	binary.LittleEndian.PutUint16(b.data[len(b.data)-2:], uint16(v))
	b.pos += 2
}

// f64 writes a double ("d"), used by Component.GetAlpha.
func (b *msgBuf) f64(v float64) {
	b.padTo(8)
	b.data = append(b.data, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64(b.data[len(b.data)-8:], math.Float64bits(v))
	b.pos += 8
}

// objPath writes an object path ("o"). On the wire it is a string; only the
// signature differs.
func (b *msgBuf) objPath(v string) { b.str(v) }

// structStart aligns for a struct ("(...)"), which D-Bus places on an 8-byte
// boundary regardless of what its fields are.
func (b *msgBuf) structStart() { b.padTo(8) }

// variant writes a complete variant: the value's signature, then the value
// itself as written by write.
func (b *msgBuf) variant(sig string, write func()) {
	b.sig(sig)
	write()
}

func (b *msgBuf) variantI32(v int32) { b.variant("i", func() { b.i32(v) }) }

// objRef is AT-SPI's universal reference to an accessible: the owning
// connection's bus name paired with the object path. Every tree navigation
// method returns one or an array of them.
type objRef struct {
	Name string
	Path string
}

// ref writes an objRef as the struct "(so)".
func (b *msgBuf) ref(r objRef) {
	b.structStart()
	b.str(r.Name)
	b.objPath(r.Path)
}

// refArray writes "a(so)".
func (b *msgBuf) refArray(refs []objRef) {
	lenPos, contentPos := b.arrayStart(8)
	for _, r := range refs {
		b.ref(r)
	}
	b.arrayEnd(lenPos, contentPos)
}

// strArray writes "as".
func (b *msgBuf) strArray(vs []string) {
	lenPos, contentPos := b.arrayStart(4)
	for _, v := range vs {
		b.str(v)
	}
	b.arrayEnd(lenPos, contentPos)
}

// u32Array writes "au", which is how AT-SPI transmits a state set: two 32-bit
// words of bit flags.
func (b *msgBuf) u32Array(vs []uint32) {
	lenPos, contentPos := b.arrayStart(4)
	for _, v := range vs {
		b.u32(v)
	}
	b.arrayEnd(lenPos, contentPos)
}

// strDict writes "a{ss}". Dict entries are structs, so each is 8-byte aligned.
func (b *msgBuf) strDict(kv map[string]string, order []string) {
	lenPos, contentPos := b.arrayStart(8)
	for _, k := range order {
		v, ok := kv[k]
		if !ok {
			continue
		}
		b.padTo(8)
		b.str(k)
		b.str(v)
	}
	b.arrayEnd(lenPos, contentPos)
}

// --- extra decoding ---------------------------------------------------------

// readI32 reads a signed 32-bit integer argument.
func (d *msgDecoder) readI32() (int32, error) {
	v, err := d.readU32()
	return int32(v), err
}

// --- server-side message encoders -------------------------------------------

// dbusEncodeReturn builds a METHOD_RETURN. dest is the caller's unique name,
// taken from the incoming message's SENDER field.
func dbusEncodeReturn(serial, replyTo uint32, dest, bodySig string, body []byte) []byte {
	hdr := newMsgBuf(16)
	dbusWriteHdrField(hdr, dbusFieldReplySerial, "u", func() { hdr.u32(replyTo) })
	if dest != "" {
		dbusWriteHdrField(hdr, dbusFieldDest, "s", func() { hdr.str(dest) })
	}
	if bodySig != "" {
		dbusWriteHdrField(hdr, dbusFieldSignature, "g", func() { hdr.sig(bodySig) })
	}
	return dbusAssembleMsg(dbusMsgReturn, 0, serial, hdr.data, body)
}

// dbusEncodeError builds an ERROR reply carrying a single string message,
// which is the shape every D-Bus error takes in practice.
func dbusEncodeError(serial, replyTo uint32, dest, errName, message string) []byte {
	body := newMsgBuf(0)
	body.str(message)

	hdr := newMsgBuf(16)
	dbusWriteHdrField(hdr, dbusFieldErrorName, "s", func() { hdr.str(errName) })
	dbusWriteHdrField(hdr, dbusFieldReplySerial, "u", func() { hdr.u32(replyTo) })
	if dest != "" {
		dbusWriteHdrField(hdr, dbusFieldDest, "s", func() { hdr.str(dest) })
	}
	dbusWriteHdrField(hdr, dbusFieldSignature, "g", func() { hdr.sig("s") })
	return dbusAssembleMsg(dbusMsgError, 0, serial, hdr.data, body.data)
}

// dbusEncodeSignal builds a SIGNAL. Signals are broadcast, so unlike a reply
// there is no destination — the bus routes them to whoever has a match rule.
func dbusEncodeSignal(serial uint32, path, iface, member, bodySig string, body []byte) []byte {
	hdr := newMsgBuf(16)
	dbusWriteHdrField(hdr, dbusFieldPath, "o", func() { hdr.objPath(path) })
	dbusWriteHdrField(hdr, dbusFieldInterface, "s", func() { hdr.str(iface) })
	dbusWriteHdrField(hdr, dbusFieldMember, "s", func() { hdr.str(member) })
	if bodySig != "" {
		dbusWriteHdrField(hdr, dbusFieldSignature, "g", func() { hdr.sig(bodySig) })
	}
	return dbusAssembleMsg(dbusMsgSignal, 0, serial, hdr.data, body)
}
