# Frontend dependency audit

Elke PR moet `bun audit --audit-level=high` (zie `.github/workflows/security.yml`,
job `Frontend dependency audit`) groen houden. Een advies met een beschikbare fix
los je op met een regel in `overrides` in `web/package.json`, niet met `--ignore`.
`--ignore` blijft alleen voor adviezen waarvoor nog geen uitgave met een fix bestaat,
met de reden erbij in `security.yml`.

## Besluiten 29 oktober 2026 (HIS-5900)

Vier adviezen op `main` (`d5bd15e9`) kregen een fix binnen hun bestaande
semver-range. Beide overrides hieronder zijn dus geen major-bump.

| Pakket | Was | Nu | Advies | Herkomst | Besluit |
|---|---|---|---|---|---|
| `seroval` | 1.6.3 / 1.6.9 | 1.6.12 | GHSA-97r2-qh9m-474h, GHSA-wp97-x4hv-qvhp, GHSA-7rv5-mrxv-xcr9 (DoS en deserialisatie via `fromJSON`) | `@tanstack/react-router` (prod) en `@tanstack/router-plugin` / `router-generator` (dev) | upgraden |
| `systeminformation` | 5.33.0 | 5.33.18 | GHSA-3f7j-7jwc-fxfj (Windows-commando-injectie in `services()`) | `shadcn` (dev) via `@dotenvx/dotenvx` | upgraden |

Beide pakketten zitten alleen onder een dev- of transitieve afhankelijkheid; de
override is de goedkoopste weg naar een groene check zonder de parentpakketten te
bumpen. Herhaal deze meting wanneer een van de parentpakketten zijn range verlaagt.
