# 🚆 Watcher — O'zbekiston temir yo'llari bilet kuzatuvchisi

Ko'p foydalanuvchili platforma. Foydalanuvchi Telegram orqali kiradi, so'ng yo'nalish va sana
bo'yicha kuzatuv yaratadi. Bo'sh joy chiqqanda Telegram'ga xabar keladi. Sotib olish rasmiy saytda qilinadi.

## Imkoniyatlar

- **Telegram orqali kirish** (rasmiy Telegram Login Widget). Telegram foydalanuvchi ma'lumotini bot tokeni bilan
  imzolaydi, server esa imzoni (HMAC-SHA256) tekshiradi, shuning uchun ma'lumotni soxtalashtirib bo'lmaydi.
  1 soatdan eski ma'lumot rad etiladi (replay himoyasi). Kirish faqat sayt login sahifasi ochilgan brauzerda
  yakunlanadi (login CSRF himoyasi). Sessiya tokeni HttpOnly cookie'da, bazada uning sha256 xeshi saqlanadi.
- **Alohida "ulash" bosqichi yo'q**: kirishda bot xabar yuborishiga ruxsat so'raladi, bildirishnomalar
  foydalanuvchining o'z Telegram'iga keladi.
- Har bir foydalanuvchi **faqat o'z kuzatuvlarini** ko'radi va boshqaradi. Egalik usecase qatlamida tekshiriladi.
- **Limitlar**: foydalanuvchiga N ta faol kuzatuv (`MAX_WATCHES_PER_USER`). Qo'lda qidiruv har bir foydalanuvchi uchun 10 soniyada bir marta.
- **Tekshiruv oraliqlari jadvali**: jo'nashga qancha qolganiga qarab, admin `/admin/polling` sahifasida boshqaradi
  (serverni qayta ishga tushirish shart emas). Boshlang'ich jadval:

  | Jo'nashgacha | Oraliq |
  |---|---|
  | 2 kundan kam (bugun, ertaga) | 10 soniya |
  | 10 kundan kam | 5 daqiqa |
  | 30 kundan kam | 1 soat |
  | qolganlari | 12 soat |

  Eng qisqa oraliq 10 soniya. Admin sahifasida har bir bosqichdagi yo'nalishlar soni va saytga taxminiy so'rovlar soni (soatiga) ko'rinadi.
- **Temir yo'l saytini qiynamaslik**:
  - bir xil yo'nalish+sana uchun qancha foydalanuvchi bo'lmasin, **bitta so'rov** yuboriladi;
  - siklda ko'pi bilan `MAX_SEARCHES_PER_CYCLE` ta so'rov yuboriladi, eng uzoq tekshirilmagan yo'nalishlar birinchi navbatda;
  - monitor har 10 soniyada uyg'onadi va faqat oralig'i o'tgan yo'nalishlarni tekshiradi;
  - so'rovlar orasida 2–4 soniya pauza bor, shuning uchun saytga ~3 soniyada bittadan ko'p so'rov ketmaydi (soatiga ~1200);
  - xato bo'lsa pauza qilinadi: kamida 1 daqiqa, keyin har safar ×2, 30 daqiqagacha, `Retry-After`ga ham amal qilinadi.

## Arxitektura (Clean Architecture)

```
cmd/watcher/                     composition root (main.go) + SQLite drayveri (driver.go)
internal/
  domain/                        entity va biznes qoidalari: User, Identity (Telegram), Watch, Route, Train, Filter, PollingPolicy
  usecase/                       stsenariylar + portlar (interfeyslar)
    ports.go                     WatchRepository, UserRepository, SessionRepository, PollingRuleRepository,
                                 TrainSearcher, IdentityVerifier, Notifier, StationDirectory, Clock
    auth_service.go              Telegram orqali kirish, sessiyalar
    telegram_service.go          bot: /start javobi, test xabar
    watch_service.go             kuzatuvlar (egalik + limit)
    search_service.go            qo'lda qidiruv (cheklovlar bilan)
    polling_service.go           oraliqlar jadvali (faqat admin) va yuklama statistikasi
    monitor.go                   davriy tekshiruv (jadval bo'yicha, guruhlash, chegara, backoff)
  adapter/
    sqlite/                      database/sql repolar + ichki SQL migratsiyalar
    telegram/                    Bot API (xabar, getUpdates) + Login Widget imzosini tekshirish
    railway/                     eticket.railway.uz API (DTO → domain)
    stations/, clock/
  delivery/
    webui/                       HTTP: sessiya middleware → handler → usecase → presenter → shablon
    telegrambot/                 Telegram'dan kiruvchi /start buyruqlari
  config/, pkg/textfmt/
```

Bog'liqlik qoidasi: `delivery → usecase → domain`. `domain` hech qaysi qatlamni import qilmaydi, `usecase` faqat `domain`ni biladi.
Tashqi kutubxonaga faqat `cmd/watcher/driver.go` bog'liq: `modernc.org/sqlite`, sof Go, cgo kerak emas. Qolgan kod standart kutubxona bilan yozilgan.

## Sozlash

### 1. Telegram bot

1. @BotFather → `/newbot` → tokenni `.env` dagi `TELEGRAM_BOT_TOKEN` ga yozing.
2. @BotFather → `/setdomain` → botingizni tanlang → saytingiz domenini yozing (masalan `watcher.example.uz`).
   **Login Widget faqat shu domenda ishlaydi**, `BASE_URL` ham aynan shu domen bo'lishi kerak (`https://`).
3. Bot long polling bilan ishlaydi, shuning uchun bot uchun webhook o'rnatilmagan bo'lishi kerak.

Lokal ishlab chiqishda widget `localhost`da ishlamaydi. Tunnel orqali vaqtinchalik https domen oling
(masalan `cloudflared tunnel --url http://localhost:8080`), shu domenni `/setdomain` va `BASE_URL` ga yozing.

### 2. Administratorlar

`ADMIN_TELEGRAM_IDS` ga adminlarning Telegram ID'larini vergul bilan yozing. O'z ID'ingizni bilish uchun @userinfobot ga yozing.
Username o'zgarishi mumkin, ID esa o'zgarmaydi, shuning uchun ID ishlatiladi.

### 3. Ishga tushirish

```bash
cp .env.example .env      # qiymatlarni to'ldiring
docker compose up -d --build
# -> http://localhost:8080
```

Docker'siz:

```bash
go mod tidy               # SQLite drayverini yuklaydi (bir marta; go.sum'ni commit qiling)
export BASE_URL=https://sizning-domen TELEGRAM_BOT_TOKEN=... ADMIN_TELEGRAM_IDS=...
go run ./cmd/watcher
```

Migratsiyalar ishga tushganda avtomatik qo'llanadi.

### Ma'lumotlar bazasi: SQLite

- Bitta fayl: `data/watcher.db`. Alohida DB server kerak emas.
- WAL rejimi, `foreign_keys` yoqilgan, `busy_timeout` 5 soniya. Barcha so'rovlar bitta ulanish orqali o'tadi, shuning uchun "database is locked" xatolari bo'lmaydi.
- **Bitta server nusxasida ishlating.** SQLite fayli bitta mashinada turadi. Bir nechta nusxa yoki gorizontal masshtab kerak bo'lsa, `usecase` portlari uchun boshqa DB adapteri yoziladi va `main.go`da almashtiriladi.
- **Zaxira nusxa** (server ishlab turganda ham xavfsiz):
  ```bash
  sqlite3 data/watcher.db ".backup data/backup-$(date +%F).db"
  ```

### Sozlamalar (env)

| O'zgaruvchi | Default | Izoh |
|---|---|---|
| `DATABASE_PATH` | `./data/watcher.db` | SQLite fayli (papkasi avtomatik yaratiladi) |
| `BASE_URL` | `http://localhost:8080` | Tashqi manzil (BotFather `/setdomain` dagi domen, `https://`). Widget callback'i va `Secure` cookie shundan |
| `TELEGRAM_BOT_TOKEN` | — (majburiy) | Kirish va bildirishnomalar shu bot orqali |
| `MAX_SEARCHES_PER_CYCLE` | `30` | Bir siklda saytga yuboriladigan so'rovlar chegarasi |
| `MAX_WATCHES_PER_USER` | `10` | Foydalanuvchiga faol kuzatuvlar soni (`0` = cheksiz) |
| `SESSION_TTL` | `720h` | Sessiya muddati |
| `ADMIN_TELEGRAM_IDS` | — | Telegram ID'lar, vergul bilan. Adminlar `/admin/polling` (oraliqlar jadvali, yuklama) va API debug sahifasini ko'radi |
| `RAILWAY_BASE`, `RAILWAY_LANG`, `RAILWAY_COOKIE` | | Temir yo'l API sozlamalari |
| `STATIONS_FILE`, `TZ_NAME`, `LISTEN_ADDR`, `DATA_DIR` | | |

## Testlar

```bash
go test -race ./...
```

Domen va usecase'lar soxta (fake) portlar bilan, tarmoqsiz va bazasiz tekshiriladi. Testlar qamrab olgan holatlar:
Telegram imzosi (soxta, boshqa token, o'zgartirilgan maydon), eskirgan kirish ma'lumoti, sessiya muddati, egalik (A ning kuzatuvini B o'zgartira olmaydi), limitlar,
Telegram kodining bir martaligi va muddati, bitta chatning ikki akkauntga bog'lanmasligi,
bir xil yo'nalishga bitta so'rov, so'rovlar chegarasi va navbat, `Retry-After`, sanasi o'tgan kuzatuvlar.

## ⚠️ Birinchi ishga tushirishda tekshiring

API rasmiy hujjatlashtirilmagan:
1. **Javob maydonlari**: `internal/adapter/railway/dto.go`. Admin sifatida bitta qidiruv qiling va "Oxirgi javob" sahifasini oching.
2. **Stansiya kodlari**: `internal/adapter/stations/stations.json`. Toshkent `2900000` aniq, qolganlarini saytdagi so'rovlar bilan solishtiring.

## Eslatma

Bu ichki (rasmiy bo'lmagan) API. Format istalgan payt o'zgarishi mumkin, ko'p so'rov esa IP blokka olib keladi.
Ochiq platforma sifatida ishga tushirishdan oldin O'ztemiryo'lyo'lovchi AJ bilan rasmiy kelishuv tavsiya etiladi (e-ticket@railway.uz).
