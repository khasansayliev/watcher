# 🚆 railbot — O'zbekiston temir yo'llari bilet kuzatuvchisi

Yo'nalish va sana tanlab kuzatuv yaratasiz. Servis vaqti-vaqti bilan bo'sh joylarni tekshiradi
va mos joy chiqsa Telegram'ga xabar yuboradi. Sotib olish rasmiy saytda qilinadi.

Faqat Go standart kutubxonasi ishlatilgan: tashqi bog'liqlik ham, DB ham yo'q (kuzatuvlar `data/watches.json` faylida saqlanadi).

## Ishga tushirish

```bash
# Go 1.22+ kerak
go build -o railbot .

export TELEGRAM_BOT_TOKEN="123456:ABC..."     # @BotFather'dan
export TELEGRAM_DEFAULT_CHAT_ID="123456789"   # ixtiyoriy, formada avtomatik to'ladi
export UI_PASSWORD="kuchli-parol"             # serverga qo'ysangiz albatta bering
./railbot
# -> http://localhost:8080
```

Docker bilan:

```bash
docker build -t railbot .
docker run -d -p 8080:8080 -v $PWD/data:/app/data \
  -e TELEGRAM_BOT_TOKEN=... -e UI_PASSWORD=... railbot
```

## Sozlamalar (env)

| O'zgaruvchi | Default | Izoh |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | — | Bot tokeni. Bo'lmasa xabar yuborilmaydi |
| `TELEGRAM_DEFAULT_CHAT_ID` | — | Formadagi chat ID uchun default qiymat |
| `POLL_INTERVAL` | `5m` | Tekshiruv oralig'i. Eng kami `2m` |
| `LISTEN_ADDR` | `:8080` | UI manzili |
| `DATA_DIR` | `./data` | `watches.json` va `last_response.json` shu yerda saqlanadi |
| `UI_PASSWORD` / `UI_USER` | — / `admin` | UI uchun Basic Auth |
| `RAILWAY_BASE` | `https://eticket.railway.uz` | API bazaviy manzili |
| `RAILWAY_LANG` | `uz` | |
| `RAILWAY_COOKIE` | — | Avtomatik token olinmasa: brauzerdagi cookie qatori (`XSRF-TOKEN=...; ...`) |
| `STATIONS_FILE` | ichki `stations.json` | Stansiyalar ro'yxatini almashtirish uchun |

## Telegram chat ID

1. Botingizga `/start` yozing (bot sizga birinchi bo'lib yoza olmaydi).
2. @userinfobot ga yozsangiz, ID'ingizni aytadi.
3. Guruh uchun: botni guruhga qo'shing. Guruh ID'si manfiy son bo'ladi (`-100...`).
4. UI'dagi "Telegram'ni tekshirish" tugmasi orqali test xabar yuborib ko'ring.

## Qanday ishlaydi

- Bir xil yo'nalish va sanadagi kuzatuvlar uchun **bitta so'rov** yuboriladi.
- So'rovlar orasida 2–4 soniya pauza bor, oraliqqa ±10% tasodifiy og'ish qo'shiladi.
- Xato kelsa (429, 403, 5xx, tarmoq), tekshiruv to'xtab turadi. Pauza oraliq → ×2 → … → 30 daqiqagacha oshadi, `Retry-After` header'iga ham amal qilinadi.
- Xabar **faqat mos joylar to'plami o'zgarganda** yuboriladi: yangi poyezd yoki vagon turi chiqqanda. Har siklda takror yuborilmaydi. Joylar tugab, keyin qayta chiqsa, yana xabar keladi.
- Sanasi o'tgan kuzatuvlar avtomatik to'xtatiladi.
- Qo'lda qidiruv 10 soniyada bir martadan ko'p yuborilmaydi.

## ⚠️ Birinchi ishga tushirishda tekshiring

API rasmiy hujjatlashtirilmagan, shuning uchun ikki joy taxminga asoslangan:

1. **Javob maydonlari** (`railway.go` → "Javobni o'qish" bo'limi). Bitta qidiruv qiling va UI'dagi
   **Oxirgi javob** havolasini oching (yoki `data/last_response.json` faylini). Maydon nomlari boshqacha
   bo'lsa, faqat o'sha bo'limdagi `json:"..."` teglarini moslang.
2. **Stansiya kodlari** (`stations.json`). Toshkent `2900000` aniq. Qolganlari taxminiy: saytda qidiruv qilib,
   DevTools → Network → `trains/list` so'rovidagi `depStationCode`/`arvStationCode` qiymatlari bilan solishtiring.

Token avtomatik olinmasa (HTTP 403 yoki "XSRF-TOKEN olinmadi"), brauzerda saytga kiring.
DevTools → Network → istalgan API so'rovi → `Cookie` header qiymatini `RAILWAY_COOKIE` ga bering.

## Eslatma

Bu ichki (rasmiy bo'lmagan) API. Format istalgan payt o'zgarishi mumkin.
Oraliqni kamaytirmang: sayt baribir yuklamadan qiynalib turibdi, ortiqcha so'rov esa IP blokka olib keladi.
Tijoriy foydalanish uchun O'ztemiryo'lyo'lovchi AJ bilan rasmiy shartnoma kerak (e-ticket@railway.uz).
