# shortorder canvas fork

This is a narrow local fork of `github.com/tdewolff/canvas` at
`v0.0.0-20260508100355-63a7228e682d`. The upstream source is MIT licensed; see
`LICENSE.md`.

Local changes:

- `text/fribidi.go` replaces upstream's LGPL-2.1 pure-Go FriBidi fallback.
- `text/fribidi_cgo.go` is removed so `-tags=fribidi` cannot reintroduce a
  system FriBidi dependency.
- `latex.go` disables the embedded Latin Modern implementation. shortorder
  builds with `-tags=latex`, which selects `latex_bin.go` instead of the
  embedded-font path.
