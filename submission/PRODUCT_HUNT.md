# Product Hunt launch listing: Stampede

> **Status: SCHEDULED** on Product Hunt for **Wednesday, Oct 14, 2026** (goes live 12:01 AM PT / 3:01 AM ET).
> Prelaunch: https://www.producthunt.com/products/stampede-2/stampede-2/prelaunch (slug `stampede-2`). Not entered in the Cloud Run hackathon.
> Scheduled 2026-10-06 as @shyam_desigan. Gallery uses `01-home.png`, `02-running.png`, `03-result.png`. Thumbnail was a temporary crop of home — replace before launch if you want a real mark.

---

## Name

**Stampede**

## Tagline (≤60 chars)

**Will your app survive the Product Hunt front page?** (50 chars)

Alternates:
- Load-test your launch before Product Hunt does (46)
- A 60-second launch-day load check for makers (44)

## Links

- Website: https://stampede-three.vercel.app
- GitHub: https://github.com/icohangar-ops/stampede

## Short description (PH "Description" field, ≤260 chars)

Find out in 60 seconds whether your app survives the Product Hunt front page. Paste a URL you own, prove ownership, pick a launch-shaped traffic curve, and watch RPS, latency, and errors live. Get a readiness report and a shareable badge.

(238 chars)

## Topics / tags

Primary (pick 3 in the PH topic picker):
1. Developer Tools
2. Web App
3. SaaS

Backups if any of those aren't available: Open Source, Productivity, Tech.

Keywords for search/socials: load testing, launch day, Product Hunt, performance, latency, Vercel, makers, indie hackers.

## Pricing

Free (open source, demo hosted on Vercel).

---

## Maker's first comment (post right after the listing goes live)

Hey Product Hunt 👋 I'm Sam, the maker of Stampede.

Every launch day I watch makers get to the front page and then their app goes down. The traffic shows up as a spike in the first couple of hours, not a smooth line, and most of us have never tested for that.

Stampede is a launch-day load check:

1. **Paste a URL you own.**
2. **Prove ownership** with a `/.well-known/stampede-<token>.txt` file or a DNS TXT record. No ownership, no run.
3. **Pick a launch-shaped curve:** "Top 5 of the Day" or "#1 Product of the Day".
4. **Watch it live:** requests per second, p95 latency, and errors stream in as the curve ramps up, until it finds your breaking point.
5. **Get a readiness report**, a Vercel cost estimate, and a **shareable badge**.

**Try it with no setup:** pick "Use the demo shop." Northwind Kits is a demo store built to fold. Past about 24 req/s its p95 goes over 1.5s, and past about 34 the homepage starts returning 503s. You'll see the breaking point show up live.

**Built to be safe, not a DDoS button:**
- Ownership verification is required before any run, and external sites are checked again when the run starts
- GET requests only
- Hard caps: 40 req/s, 50 workers, 3 minutes max (the hosted demo curve is labeled 30s; on the demo shop wall time is often 1–2 minutes)
- Daily quotas: 3 runs per external domain, 200 for the hosted demo shop (`DEMO_DOMAIN_QUOTA`), and 5 per network
- SSRF protection: private, loopback, link-local, CGNAT, and cloud-metadata addresses are blocked, and redirects have to stay on the same hostname

**Honest caveats:** the curves are planning shapes scaled down to the safety cap, not a real Product Hunt traffic feed. A green badge doesn't guarantee your app survives uncapped #1 traffic. The cost figure is an estimate, not an invoice.

It's open source and runs on Vercel: https://github.com/icohangar-ops/stampede

I'd love your feedback, especially from anyone who has launched here: which curve shapes should I add, and what should the report tell you that it doesn't yet?

## Follow-up comment template (reply to early questions)

> Thanks for trying it! Quick note: Stampede only tests sites you've verified, and only with GET requests under a 40 req/s cap, so it can't be pointed at someone else's app. The demo shop allows 200 runs a day; other domains stay at 3, and each network is limited to 5. Clone the repo and run it locally (`npm run dev:api`) to keep going.

---

## Gallery images (upload in this order)

| # | File | Caption |
| --- | --- | --- |
| 1 | `01-home.png` | Paste a URL you own, or use the demo shop. Find out in 60 seconds whether it survives the front page. |
| 2 | `02-running.png` | Watch it live: requests per second, p95 latency, and errors stream in as a Product Hunt-shaped curve ramps up. |
| 3 | `03-result.png` | Get a readiness report with your breaking point, a Vercel cost estimate, and a shareable badge. |

Thumbnail: Stampede logo/mark, 240×240 (GIF allowed).
Recommended gallery size: 1270×760.

---

## Launch-day checklist (Wed, Oct 14, 2026, ET)

**Before launch (by Oct 13)**
- [x] Scheduled for 12:01 AM PT / 3:01 AM ET Oct 14 (confirmed “Successfully Scheduled!”). Prelaunch: https://www.producthunt.com/products/stampede-2/stampede-2/prelaunch
- [x] Capture gallery images `01-home.png`, `02-running.png`, `03-result.png` (uploaded to PH; resize to 1270×760 still optional).
- [x] Do a full demo run on https://stampede-three.vercel.app (Use the demo shop → verify → Top 5 of the Day). Sample result: https://stampede-three.vercel.app/r/0d0a0e3340df871a189af7015f10568b
- [x] **Check demo quota capacity.** External domains stay at 3 runs per UTC day. The embedded `/demo` shop (and `stampede-three.vercel.app/demo`) uses `DEMO_DOMAIN_QUOTA`, default 200. IP quota stays 5.
- [ ] Set a real `STAMPEDE_SECRET` and a `STAMPEDE_ADMIN_TOKEN` in Vercel. Confirm `KILL_SWITCH=1` stops runs, then set it back.
- [ ] Check the Vercel usage/spend limits and turn on alerts.
- [ ] Make sure the README, the repo description, and the social preview image all match the listing.

**Launch morning**
- [ ] 3:01 AM ET: confirm the listing is live and post the maker's first comment right away.
- [ ] Share the link with your own network and communities (X, LinkedIn, maker groups). Ask for feedback, not upvotes.
- [ ] Do a smoke-test run on the demo every couple of hours.

**During the day**
- [ ] Reply to every comment within about 30 minutes.
- [ ] Watch Vercel runtime logs and errors for 429s, 5xxs, and function timeouts. Keep the kill switch ready.
- [ ] Post a mid-day update comment (for example, how many runs and the most common breaking points).

**After**
- [ ] Thank supporters in a closing comment, then add the PH badge and results to the README.
- [ ] Log the feedback as GitHub issues.

---

_Note: Stampede is **not** being entered in the Google Cloud Run hackathon. The live product is hosted on Vercel. Leave out any hackathon or Cloud Run references in the listing._
