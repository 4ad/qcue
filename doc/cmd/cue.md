# `cue` command reference

This fork builds an executable named `cue`; its Go module path is still
`cuelang.org/go`. From the repository root:

```sh
go install ./cmd/cue
cue version
```

Ensure the installed binary is on your `PATH`. To run this checkout directly
without installing it, use `go run ./cmd/cue` in place of `cue`.

`cue help` gives information about the various `cue` commands available, as well
as additional help topics. For example `cue help import` gives information about
how to convert other formats like JSON and YAML to CUE, and `cue help filetypes`
describes the `cue` command's support for various file types.

Useful commands include:

```sh
cue eval example.cue           # Inspect values and remaining constraints.
cue vet -c=false example.cue    # Validate without requiring concrete data.
cue vet -c example.cue          # Also require concrete regular fields.
cue export example.cue -e out  # Export the selected concrete data as JSON.
cue fmt example.cue            # Format source in place.
cue help cmd                   # Run commands defined in _tool.cue files.
cue help experiments           # Inspect language experiment settings.
```

Universal types and functions are enabled by default in this fork. Evaluation
can leave data or proof obligations incomplete; exporting a selected result
requires concrete data. Function values and unapplied function interfaces are
not JSON data. The [implementation guide](../implementation.md) explains
validation and refinement, and the [type reference](../types.md) documents
builtins, operators, standard-library functions, and tool-task schemas.
