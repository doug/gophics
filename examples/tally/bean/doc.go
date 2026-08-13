// Package bean is an independent, permissively-licensed implementation of the
// beancount plain-text accounting format: parsing, transaction balancing,
// account balances, and price conversion.
//
// # Why this exists
//
// Every mature Go beancount engine descends from one GPL-2.0 lineage. GPL code
// cannot ship inside an App Store application — the store's DRM and licence terms
// impose exactly the "further restrictions" the GPL forbids, which is why VLC was
// pulled from the store in 2011 despite being free. An app that wants to be both
// open source and distributable through Apple's and Google's stores therefore
// needs an engine under a permissive licence. This package is that engine, under
// Apache-2.0, matching the rest of this tree.
//
// # Provenance
//
// A file format is not copyrightable — only a particular implementation of one is
// — so an independent implementation is legitimate as long as it copies no code.
// This package was written from the beancount syntax documentation and from
// permissively-licensed references (the MIT tree-sitter-beancount grammar, the
// MIT/Apache beancount-parser-lima, and knut's Apache-2.0 valuation approach). No
// GPL implementation was copied. A GPL engine may be used during development as a
// black-box oracle — run it, compare the answers — which is testing, not copying;
// the fixtures committed here state their expected values directly.
//
// # Scope
//
// Deliberately not a complete beancount: this covers the directives real personal
// ledgers use, and grows as they demand more. Not implemented (yet): the query
// language, plugin execution, and the more exotic corners of lot booking. Where
// behaviour is intentionally narrower than the reference implementation, the doc
// comment says so rather than pretending.
package bean
