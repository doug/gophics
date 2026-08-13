# Store listing — Tally

Draft copy and the answers the review forms ask for. Written to be true: every
claim below is checkable against the source, because a finance app's whole pitch
is that you can check it.

## Name and subtitle

- **Name**: Tally
- **Subtitle** (30 chars): `Plain-text money, your file`
- **Promotional text**: Your finances in a file you own — readable in any editor,
  backed up however you like, and never sent anywhere.

## Description

> Tally is a personal-finance app with an unusual property: your data is a plain
> text file on your own disk, in the beancount format, and Tally never sends it
> anywhere. There is no account to create, no server to trust, and no export
> button — because nothing was ever locked in.
>
> **See where you stand.** Net worth over time, with investments valued at their
> recorded prices rather than counted as the cash you spent on them. Income
> against expenses month by month. Where the money actually goes, largest first.
>
> **Read every account.** Drill into any account for a full register: dates,
> payees, the other side of each transaction, and a running balance in tabular
> figures that line up so columns are readable at a glance.
>
> **Add transactions.** Record what you spent and Tally writes it into your
> ledger — in date order, leaving your comments, headings and formatting exactly
> as you left them. If an entry makes one of your balance checks no longer match,
> Tally tells you rather than letting you find out later.
>
> **Own it.** The file is yours. Open it in any text editor, keep it in version
> control, sync it however you already sync things, or take it to any other
> beancount tool. Tally is one way to look at your ledger, not a place your money
> data lives.
>
> Tally is free and open source.

## Keywords

`beancount, ledger, plain text accounting, personal finance, budget, net worth,
double entry, expenses, local first, privacy`

## Category

Finance. (Secondary: Productivity.)

## Privacy answers

Both stores ask the same questions in different words. The answers are the same,
and they are answerable in one sentence: **Tally collects nothing, because Tally
has no networking code at all.**

| Question | Answer |
|---|---|
| Data collected | None |
| Data linked to the user | None |
| Data used for tracking | None |
| Third-party analytics / SDKs | None |
| Account required | No |
| Account deletion path | Not applicable — no accounts |
| Purpose strings needed | None: no camera, microphone, location, contacts, photos, or notifications |

Apple's privacy manifest (`PrivacyInfo.xcprivacy`) should declare no collected
data types and no required-reason APIs beyond file access the user initiates.

For Play's Data Safety form: "No data collected", "No data shared", and the
"Data is encrypted in transit" question is not applicable because nothing is
transmitted.

## Permissions

- **macOS sandbox**: `com.apple.security.files.user-selected.read-write` only.
- **iOS**: no entitlements beyond the default; document access is through the
  system picker, which needs none.
- **Android**: no `<uses-permission>` at all — the ledger arrives through the
  system document picker.

## Screenshots to capture

All can be produced headlessly from the app itself (`GOPHICS_THUMB`), which is
worth doing because it keeps them honest — they are the real renderer, not a
mockup.

1. Overview: net worth curve with the income/expense chart below it.
2. Where the money goes: the category table with its share bars.
3. A register: one account, dates and running balance.
4. The add-transaction form, filled in.
5. Balances: the account tree.

## Age rating

4+ / Everyone. No user-generated content, no ads, no purchases, no links out
except to the project page.

## Support and marketing URLs

Both are required. Point them at the project's repository and its README until
there is a site.

## What is *not* claimed

Worth stating plainly so the listing never oversells:

- Tally does not import from banks. Transactions are entered by hand or added to
  the file by other tools.
- Tally does not fetch prices. Investment valuations use the price directives
  already in your ledger, and holdings with no price are shown as excluded rather
  than guessed at.
- Tally does not sync. The file is where it is; use whatever you already use.
