# 99. A roman-numeral sequel searches by its bare base title first

**Location:** `nyaa/nyaa.go` (`romanSequelBase`, the first variant in
`GenerateSearchTitleVariants`); `reRomanSeasonTitle` in `nyaa/nyaa_regex.go`.

DanMachi V's titles are "…Machigatteiru Darou ka **V: Houjou no Megami-hen**" and "…in a Dungeon?
V". The releases say "…Machigatteiru Darou ka **S5** - 08". Every variant carried the numeral and
the subtitle, so the title match required tokens the releases don't have, and only dead DubOnly
rows (named after the full title) were found. `ExtractSeason` already read the "V" as season 5 —
the filter was right, the query was wrong. The base title ("…Darou ka") now goes first; the
season filter keeps the other seasons out.

**What looks too strict: the numeral counts only when the English title carries the same one as a
word.** "Lupin III" (English "Lupin the 3rd") is a name, not a season 3, and gets no base variant.
