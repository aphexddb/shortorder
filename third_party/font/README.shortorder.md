# shortorder font fork

This is a narrow local fork of `github.com/tdewolff/font` at
`v0.0.0-20260527091451-1663e68cb8a4`. The upstream source is MIT licensed; see
`LICENSE.md`.

Local changes:

- `go.mod` is pruned to the root package dependencies shortorder compiles.
  Upstream optional test/example dependencies are not needed for bundled font
  loading and kept obsolete excluded modules in the full module graph.
