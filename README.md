# 🚆 Watcher — O'zbekiston temir yo'llari bilet kuzatuvchisi

Ko'p foydalanuvchili platforma. Foydalanuvchi Telegram orqali kiradi (alohida ro'yxatdan o'tish yo'q), so'ng yo'nalish va sana
bo'yicha kuzatuv yaratadi. Bo'sh joy chiqqanda Telegram'ga xabar keladi. Sotib olish rasmiy saytda qilinadi.

## Imkoniyatlar

- **Telegram orqali kirish va ro'yxatdan o'tish**: saytdagi tugma botni kod bilan ochadi, Start bosilgach akkaunt yaratiladi/topiladi va brauzer kiradi. Parol yo'q.
- Sessiya tokeni faqat HttpOnly cookie'da turadi, bazada uning sha256 xeshi saqlanadi.
- **Jo'nash vaqti oralig'i**: kuzatuv va qidiruvda «shu soatdan keyingilar», «shu soatgacha bo'lganlar», «shu ikki soat oralig'idagilar» yoki (bo'sh qoldirilsa) kun bo'yicha hammasi. Forma standart ravishda Toshkent → Buxoro.
- **Kuzatuvni tahrirlash**: «Tahrirlash» tugmasi — yo'nalish, sana, vaqt oralig'i, narx, joy soni va tur o'zgaradi. Yaratish bilan bir xil tekshiruvlar va ruxsat qoidalari amal qiladi; o'zgargach kuzatuv darhol tekshiriladi va yangidan xabar beradi. Admin belgilari (to'xtatish, alohida oraliq) va oldingi loglar saqlanadi. Tekshiruv paytida tahrirlansa, eskirgan natija yozilmaydi.
- **Tekshiruv loglari**: har bir kuzatuv uchun har bir so'rov yoziladi (yuborilgan vaqt, davomiyligi, natija yoki xato, nechta poyezd/mos, xabar yuborildimi, saytdan kelgan poyezdlar va joylar). Kuzatuvdagi «Loglar» tugmasi orqali `req_time` bo'yicha kamayish tartibida, 25 tadan sahifalab ko'riladi. Eski loglar avtomatik o'chadi (`LOG_RETENTION_DAYS`).
- **Admin boshqaruvi** (`/admin/users`, `/admin/watches`, faqat `ADMIN_TELEGRAM_IDS` dagilar):
  - Kuzatuv yaratish rejimi: **faqat ruxsat berilganlar** (standart), **hamma** yoki **hech kim**. Yangi foydalanuvchi ruxsatsiz boshlaydi; admin har biriga ruxsat beradi yoki oladi. **Ruxsati yo'q foydalanuvchining mavjud kuzatuvlari ham tekshirilmaydi** va xabar yubormaydi (ruxsat qaytsa davom etadi). «Hech kim» rejimida adminlardan tashqari hamma kuzatuvlar to'xtaydi. Adminlar har doim ruxsatli.
  - Foydalanuvchini o'chirish/yoqish: o'chirilganning sessiyalari tugaydi, kira olmaydi, kuzatuvlari tekshirilmaydi (qayta yoqilsa davom etadi). O'zini o'chirib bo'lmaydi.
  - Kuzatuvni to'xtatish/yoqish (hamma yoki bitta foydalanuvchiniki): admin to'xtatganini foydalanuvchi o'zi yoqa olmaydi.
  - Har bir kuzatuv uchun **alohida tekshiruv oralig'i** (10 soniyadan 7 kungacha): `/admin/watches` dagi «Oraliq» formasi. Belgilanmasa umumiy jadval ishlaydi; «Jadvalga» tugmasi qaytaradi. Bir yo'nalishda bir nechta kuzatuv bo'lsa, saytga bitta so'rov ketadi, lekin har bir kuzatuv faqat o'z oralig'i kelganda baholanadi. Admin sahifasidagi yuklama jadvalida alohida oraliqlar ham hisobga olinadi.
- **Telegram bildirishnomalari**: akkaunt va chat bir xil, alohida "ulash" bosqichi yo'q.
  Kod bir martalik va 10 daqiqa amal qiladi, faqat uni ochgan brauzerga bog'langan. Chat ID'ni qo'lda kiritish yo'q, shuning uchun birovning chatiga xabar yuborib bo'lmaydi.
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
  domain/                        entity va biznes qoidalari: User, TelegramProfile, Watch, Route, Train, Filter, PollingPolicy
  usecase/                       stsenariylar + portlar (interfeyslar)
    ports.go                     WatchRepository, UserRepository, SessionRepository, LoginRequestRepository,
                                 TrainSearcher, IdentityProvider, Notifier, StationDirectory, Clock
    auth_service.go              Telegram login/signup, sessiyalar
    telegram_service.go          test xabar yuborish
    watch_service.go             kuzatuvlar (egalik + limit)
    search_service.go            qo'lda qidiruv (cheklovlar bilan)
    polling_service.go           oraliqlar jadvali (faqat admin) va yuklama statistikasi
    monitor.go                   davriy tekshiruv (jadval bo'yicha, guruhlash, chegara, backoff)
  adapter/
    sqlite/                      database/sql repolar + ichki SQL migratsiyalar
    telegram/                    Bot API: xabar yuborish, getUpdates
    railway/                     eticket.uzrailpass.uz API (DTO → domain)
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

@BotFather → `/newbot` → tokenni `.env` dagi `TELEGRAM_BOT_TOKEN` ga yozing (majburiy: bot bo'lmasa hech kim kira olmaydi).
Botga `/logout` yozilsa, shu akkauntning barcha sessiyalari tugatiladi.
Bot long polling bilan ishlaydi, shuning uchun bot uchun webhook o'rnatilmagan bo'lishi kerak.

### 2. Ishga tushirish

```bash
cp .env.example .env      # qiymatlarni to'ldiring
docker compose up -d --build
# -> http://localhost:8080
```

Docker'siz:

```bash
go mod tidy               # SQLite drayverini yuklaydi (bir marta; go.sum'ni commit qiling)
export BASE_URL=http://localhost:8080 TELEGRAM_BOT_TOKEN=...
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
| `BASE_URL` | `http://localhost:8080` | Tashqi manzil. `https://` bo'lsa cookie'lar `Secure` bo'ladi |
| `TELEGRAM_BOT_TOKEN` | — (majburiy) | Kirish va xabarlar shu bot orqali |
| `MAX_SEARCHES_PER_CYCLE` | `30` | Bir siklda saytga yuboriladigan so'rovlar chegarasi |
| `MAX_WATCHES_PER_USER` | `10` | Foydalanuvchiga faol kuzatuvlar soni (`0` = cheksiz) |
| `SESSION_TTL` | `720h` | Sessiya muddati |
| `LOG_RETENTION_DAYS` | `14` | Tekshiruv loglarini saqlash muddati, kun (`0` = o'chirmaslik). 10 soniyalik kuzatuv kuniga ~8 600 ta yozuv beradi |
| `ADMIN_TELEGRAM_IDS` | — | Adminlarning Telegram ID'lari, vergul bilan (ID'ni @userinfobot dan bilib oling). Adminlar `/admin/polling` (oraliqlar jadvali, yuklama) va API debug sahifasini ko'radi |
| `RAILWAY_BASE`, `RAILWAY_LANG`, `RAILWAY_COOKIE` | | Temir yo'l API sozlamalari |
| `STATIONS_FILE`, `TZ_NAME`, `LISTEN_ADDR`, `DATA_DIR` | | |

## Testlar

```bash
go test -race ./...
```

Domen va usecase'lar soxta (fake) portlar bilan, tarmoqsiz va bazasiz tekshiriladi. Testlar qamrab olgan holatlar:
Telegram orqali login (bir martalik, muddatli kod) va sessiya muddati, egalik (A ning kuzatuvini B o'zgartira olmaydi), limitlar,
kirish kodining bir martaligi va muddati, sessiyalarni `/logout` bilan tugatish,
bir xil yo'nalishga bitta so'rov, so'rovlar chegarasi va navbat, `Retry-After`, sanasi o'tgan kuzatuvlar.

## ⚠️ Birinchi ishga tushirishda tekshiring

API rasmiy hujjatlashtirilmagan:
1. **Javob maydonlari**: `internal/adapter/railway/dto.go`. Admin sifatida bitta qidiruv qiling va "Oxirgi javob" sahifasini oching.
2. **Stansiya kodlari**: `internal/adapter/stations/stations.json`. Toshkent `2900000` aniq, qolganlarini saytdagi so'rovlar bilan solishtiring.

## Eslatma

Bu ichki (rasmiy bo'lmagan) API. Format istalgan payt o'zgarishi mumkin, ko'p so'rov esa IP blokka olib keladi.
Ochiq platforma sifatida ishga tushirishdan oldin O'ztemiryo'lyo'lovchi AJ bilan rasmiy kelishuv tavsiya etiladi (e-ticket@railway.uz).
