# WaCalls upstream

The call signaling, relay, SRTP, RTP, PCM, and MLow implementation in this
directory is derived from [JotaDev66/WaCalls](https://github.com/JotaDev66/WaCalls)
at commit `edeb31f0427aba896639db503153b777a405eccf`.

WaCalls is distributed under the MIT License. Its license is preserved in
`internal/wacalls/LICENSE`. The MLow implementation carries its own MIT license
in `internal/wacalls/media/mlow/LICENSE`.

Evolution Go adaptations are limited to module import paths, the package name
of the WhatsApp socket adapter, and integration with the existing Evolution Go
`whatsmeow.Client` lifecycle.
