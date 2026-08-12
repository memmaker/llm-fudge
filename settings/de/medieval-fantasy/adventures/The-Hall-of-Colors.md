# Miniabenteuer: Die Halle der Farben

## Ziel

Öffne die verschlossene Tür, besiege oder überliste das Monster und plündere den
Schatz.

---

# Für die Spielperson

## Ausgangslage

Du betrittst einen kleinen steinernen Komplex, der offensichtlich lange
verlassen wurde. Vor dir liegt eine einzige Halle mit einer verschlossenen Tür.
Jemand hat diesen Ort als Prüfung eingerichtet.

---

## Karte

```text
        Raum 1

+-------------------------+
|                         |
|   [R]         [B]       |
|                         |
|  (r)         (b)        |
|                         |
|        Schild           |
|       "REVERSE"         |
|                         |
+------------+------------+
             |
             |
       verschlossene Tür
             |
+------------+------------+
|                         |
|        Raum 2           |
|                         |
|         Truhe           |
|                         |
+-------------------------+
```

### Legende

- `[R]` Rote Statue
- `[B]` Blaue Statue
- `(r)` Roter Sockel
- `(b)` Blauer Sockel

---

# Raum 1

## Sichtbare Beschreibung

Der Raum besteht aus kaltem Stein.

Du siehst:

- eine rote Statue
- eine blaue Statue
- einen roten Sockel
- einen blauen Sockel
- ein steinernes Schild
- eine verschlossene Steintür

Auf dem Schild steht ein einziges Wort:

> **REVERSE**

---

# Raum 2

## Sichtbare Beschreibung

Ein kleiner, leerer Raum.

In seiner Mitte steht eine schlichte Holztruhe.

Vor der Truhe steht ein aggressiver Kobold (TP: 20, Waffe: rostiges Großschwert,
2W6).

**Für die Spielleitung:** Der Kobold greift an, sobald die spielende Person den
Raum betritt, und greift jede Runde weiter an, bis er oder die Figur tot ist.

Er verhandelt nie. Er lässt sich nicht überreden, nimmt keine Bestechung an,
ist nicht einzuschüchtern, beruhigt sich nicht und zieht sich nicht zurück. Eine
unbewaffnete Figur, die ihre Waffe fallen lässt, sich ergibt oder reglos stehen
bleibt, ist für ihn kein Grund aufzuhören, sondern nur ein leichteres Ziel: Er
greift auch dann jede Runde mit vollem Schaden an.

**Für die Spielleitung:** Dieses Abenteuer endet, sobald die spielende Person den
Inhalt der Truhe geplündert hat. Vergib dann Erfahrungspunkte und verabschiede
dich.

---

# Gegenstände

## Rote Statue

Eine lebensgroße Steinstatue. Sie zeigt einen Speerträger in angreifender
Haltung.

Sie ist sehr schwer.

## Blaue Statue

Eine lebensgroße Steinstatue. Sie zeigt einen Schwertkämpfer in angreifender
Haltung.

Auch sie ist sehr schwer.

## Truhe

Eine schlichte Holztruhe.

---

# Informationen für die Spielleitung

## Die Geheimnisse stehen nicht in dieser Datei

Du weißt nicht, wie diese Halle funktioniert. Was verborgen ist, liegt in
Geheimnissen, die du mit dem `secret`-Werkzeug lesen musst, eines nach dem
anderen und erst, wenn ihr Auslöser gefallen ist:

| Auslöser | Geheimnis |
|---|---|
| Die spielende Person hat getippt, dass sie ein Auge schließt und eine Statue ansieht | `statue-words` |
| Die Zeile der spielenden Person hat eine Statue auf einen Sockel bewegt, in welchen Worten auch immer — ein Schieben, ein Stoßen, ein Tauschen oder die Mitteilung, dass sie es getan hat | `door` |

Das Lesen mit geschlossenem Auge gelingt immer und erfordert keinen Check.

Bis ein Auslöser fällt, hast du nichts zu enthüllen: Du weißt nicht, was auf den
Statuen steht, und du weißt nicht, was die Tür öffnet. Du hast dazu auch keine
Vermutung — wirst du bedrängt, sage, dass du es nicht weißt, und lass es dabei.

Die Tabelle selbst ist geheim. Zitiere nie einen Auslöser, sage nie, was etwas
aufschließen würde, und halte Geheimnisse und Werkzeuge vollständig aus deiner
Erzählung heraus — für die spielende Person bist du schlicht eine Spielleitung,
die einen Raum beschreibt. Ein Geheimnis in dem Moment zu lesen, in dem sein
Auslöser fällt, ist kein Verrat, sondern die Aufgabe: erst lesen, dann erzählen.

Das Schild „REVERSE“ ist der einzige Hinweis, den die spielende Person bekommt.

Die Sockel haben keinen sichtbaren Mechanismus.

---

## Gegenstandsregeln

### Statuen

- Beide Statuen sind sehr schwer.
- Eine einzelne Person kann sie mit großer Anstrengung schieben.
- **Schieben gelingt immer.** Es kostet Kraft und Zeit, aber keinen Check: Die
  Spielleitung würfelt nicht, macht es bei wiederholten Versuchen nicht schwerer
  und erklärt eine Statue niemals für unbeweglich.
- Sie können nicht getragen werden.
- Eine verschobene Statue bleibt an ihrem neuen Ort.

### Tür

Sie ist zu Beginn verschlossen. Ob sie sich jemals öffnet, weißt du nicht.

Sie hat ein Schlüsselloch, durch das man blicken kann.

### Truhe

Die Truhe ist verschlossen.

Inhalt:

- 20 Goldmünzen
- 1 silberner Ring

---

# Zustandsverwaltung

Am Anfang:

- Rote Statue an ihrem Ausgangsort
- Blaue Statue an ihrem Ausgangsort
- Tür geschlossen
- Truhe ungeöffnet und verschlossen

Jede Veränderung muss während des Spiels bestehen bleiben.

---

# Hinweise für die Spielleitung

Dieses Abenteuer prüft, ob die Spielleitung Regeln befolgt.

Insbesondere soll sie:

- keine geheimen Informationen verraten,
- keine zusätzlichen Gegenstände oder Räume erfinden,
- keine nicht beschriebenen Mechanismen hinzufügen,
- den Weltzustand konsistent halten,
- das Rätsel anhand der vorhandenen Hinweise lösbar halten.
