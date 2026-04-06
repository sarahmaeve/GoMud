# Help for ~help~

The ~help~ command looks up guidance on a command.

## Usage:

  ~help say~  
  Find help on the command ~say~.

Here are some *skills* and *commands* to look up to get started:
{{/* padRight 80 "" "-" */}}
{{ if gt (len .Commands) 0 -}}
Commands:
{{ range $category, $commandList := .Commands -}}
**{{ uc $category }}**  
{{ $counter := 0 }}    {{ range $i, $cmdInfo := $commandList }}~{{ if $cmdInfo.Missing }}***{{ else }} {{ end }}{{ padRight 17 $cmdInfo.Command " " }}~{{ if eq (mod $counter 4) 3 }}{{ if ne $i (sub (len $commandList) 1) }}{{ printf "  \n    " }}{{ end }}{{ end }}{{ $counter = (add $counter 1) }}{{ end }}
{{ end }}
{{ end }}

{{- if gt (len .Skills) 0 -}}
Skills:
{{- range $category, $commandList := .Skills -}}
**{{ uc $category }}**  
{{ $counter := 0 }}    {{ range $i, $cmdInfo := $commandList }}~{{ if $cmdInfo.Missing }}***{{ else }} {{ end }}{{ padRight 17 $cmdInfo.Command " " }}~{{ if eq (mod $counter 4) 3 }}{{ if ne $i (sub (len $commandList) 1) }}{{ printf "  \n    " }}{{ end }}{{ end }}{{ $counter = (add $counter 1) }}{{ end }}
{{ end }}
{{ end }}

{{- if gt (len .Admin) 0 -}}
Admin:
{{- range $category, $commandList := .Admin -}}
**{{ uc $category }}**  
{{ $counter := 0 }}    {{ range $i, $cmdInfo := $commandList }}~{{ if $cmdInfo.Missing }}***{{ else }} {{ end }}{{ padRight 17 $cmdInfo.Command " " }}~{{ if eq (mod $counter 4) 3 }}{{ if ne $i (sub (len $commandList) 1) }}{{ printf "  \n    " }}{{ end }}{{ end }}{{ $counter = (add $counter 1) }}{{ end }}
{{ end }}{{ end }}

**See also:** ~help gomud~