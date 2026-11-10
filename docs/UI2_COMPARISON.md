# UI-biblioteket: før og nå

Sammenlignet `main` (`0377214`) med `feat/ui2-native-templ` (`93f5f9f`).
Tallene gjelder håndskrevne `.templ`-filer i henholdsvis `internal/ui` og
`internal/ui2`, inkludert delte skjemafelter; generert Go-kode er utelatt.

| | Før | Nå |
|---|---|---|
| Delte templ-filer | 13 | 9 |
| Linjer i disse filene | 1 277 | 725 |
| Komponenter og skall | `internal/ui` med egne varianter for kort, knapper, felt og tabeller | Én `internal/ui2` med skall, ikoner og få delte byggeklosser |
| Styling | Tailwind-bygg og utility-klasser | Fire vanlige CSS-kildefiler, satt sammen av `make css-build` |

Det er **4 færre delte templ-filer og 552 færre linjer (43 %)** i det
håndskrevne komponentlaget. Alle app-sider bruker nå UI2; innlogging beholder
et eget skall uten navigasjon. Sidespesifikke skjemaer og datarader ligger
fortsatt i modulene. Etterlevelsesdekning bruker native `<meter>`, faktisk fremdrift
`<progress>`, og enkle vis/skjul-felt `<details>`. Den doble pilotruten for
risikovurdering er fjernet.

**Ikke alt er blitt mindre:** gammel `input.css` hadde 33 håndskrevne linjer,
mens de fire nye CSS-filene har 1 242. Gevinsten her er bortfall av
Tailwind-bygget og én felles UI-pakke, ikke en påstand om mindre CSS.
HTMX og Alpine beholdes der eksisterende interaksjoner trenger dem.
