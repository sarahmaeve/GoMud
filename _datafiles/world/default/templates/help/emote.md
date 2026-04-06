# Help for ~emote~

The ~emote~ command is a simple role playing command that lets you customize an action or reaction to the room.

Example:

    [HP:10/10 MP:10/10]: emote scratches his head.
    **Chuckles** *scratches his head.*
    
What others see:

    [HP:6/6 MP:8/8]: **Chuckles** *scratches his head.*

Here are some shortcut emotes that can be invoked with a single word:

{{ $counter := 0 -}}{{ range $command, $output := . }}   ~{{ padRight 8 $command }}~ {{ if eq (mod $counter 6) 5 }}{{ printf "  \n" }}{{ end }}{{ $counter = (add $counter 1) }}{{ end }}