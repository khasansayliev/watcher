#!/usr/bin/env python3
# SQLite sxemasi va adapterdagi barcha so'rovlarni haqiqiy SQLite'da tekshiradi (Go drayverisiz).
# So'rov matnlari internal/adapter/sqlite/*.go fayllaridan avtomatik olinadi.
# Ishga tushirish: python3 scripts/check_sql.py
import re, sqlite3, glob, time, os
src = {f: open(f).read() for f in glob.glob('internal/adapter/sqlite/*.go')}
allsrc = "\n".join(src.values())
consts = dict(re.findall(r'const (\w+) = `([^`]*)`', allsrc))
def q(fragment):
    """Go faylidan fragment bilan boshlanadigan SQL'ni topadi va konstantalarni o'rniga qo'yadi."""
    for m in re.finditer(r'`([^`]*)`(?:\s*\+\s*(\w+)(?:\s*\+\s*`([^`]*)`)?)?', allsrc):
        s = m.group(1) + (consts[m.group(2)] if m.group(2) in consts else '') + (m.group(3) or '')
        if fragment in s: return s
    raise SystemExit("topilmadi: " + fragment)

db = sqlite3.connect(':memory:', isolation_level=None)
db.execute('PRAGMA foreign_keys=ON')
db.execute(q('CREATE TABLE IF NOT EXISTS schema_migrations'))
for f in sorted(glob.glob('internal/adapter/sqlite/migrations/*.sql')):
    v = os.path.basename(f)[:-4]
    db.execute('BEGIN'); 
    assert db.execute(q('SELECT count(*) FROM schema_migrations'), (v,)).fetchone()[0] == 0
    db.executescript(open(f).read()) if False else [db.execute(s) for s in [x for x in open(f).read().split(';') if x.strip() and not x.strip().startswith('--') or ('CREATE' in x or 'INSERT' in x)] if s.strip()]
    db.execute(q('INSERT INTO schema_migrations'), (v, int(time.time()*1000)))
    db.execute('COMMIT'); print('migratsiya OK:', v)

now = int(time.time()*1000)
up = q('INSERT INTO users (google_sub')
u1 = db.execute(up, ('g-1','ali@x.uz','Ali','', now)).fetchone()
u1b = db.execute(up, ('g-1','ali@x.uz','Ali Valiyev','', now+1)).fetchone()
u2 = db.execute(up, ('g-2','vali@x.uz','Vali','', now)).fetchone()
assert u1[0] == u1b[0] and u1b[3] == 'Ali Valiyev', (u1, u1b); print('upsert OK:', u1b)
assert db.execute(q('FROM users WHERE id = ?1'), (u2[0],)).fetchone()[2] == 'vali@x.uz'

clear, setc = q('UPDATE users SET telegram_chat_id = NULL WHERE'), q('UPDATE users SET telegram_chat_id = ?2')
for uid in (u1[0], u2[0]):
    db.execute('BEGIN'); db.execute(clear, ('555', uid)); db.execute(setc, (uid, '555')); db.execute('COMMIT')
print('chat ko\'chishi OK:', db.execute('SELECT id, telegram_chat_id FROM users ORDER BY id').fetchall())

screate = q('INSERT INTO sessions')
db.execute(screate, (b'\xaa', u1[0], now, now+86400000)); db.execute(screate, (b'\xbb', u1[0], now, now-1))
sget = q('FROM sessions s JOIN users u')
assert db.execute(sget, (b'\xaa', now)).fetchone()[0] == u1[0] and db.execute(sget, (b'\xbb', now)).fetchone() is None
assert db.execute(q('DELETE FROM sessions WHERE expires_at'), (now,)).rowcount == 1; print('sessiyalar OK')

db.execute(q('INSERT INTO telegram_link_codes'), ('abc', u1[0], now+900000))
cons = q('DELETE FROM telegram_link_codes WHERE code')
assert db.execute(cons, ('abc', now)).fetchone()[0] == u1[0] and db.execute(cons, ('abc', now)).fetchone() is None; print('link kod bir martalik OK')

wc = q('INSERT INTO watches')
w1 = db.execute(wc, (u1[0],'2900000','2900700','2026-12-05',300000.0,2,'Afrosiyob',1,now)).fetchone()
w2 = db.execute(wc, (u2[0],'2900000','2900700','2026-12-05',0.0,0,'',1,now)).fetchone()
print('watch:', w1)
assert w1[4] == '2026-12-05' and w1[8] == 1 and w1[10] is None
act = db.execute(q('WHERE active = 1 ORDER BY last_check NULLS FIRST')).fetchall(); assert len(act) == 2
assert db.execute(q('SELECT count(*) FROM watches WHERE user_id'), (u1[0],)).fetchone()[0] == 1
assert len(db.execute(q('WHERE user_id = ?1 ORDER BY created_at DESC'), (u1[0],)).fetchall()) == 1
db.execute('BEGIN'); db.execute(q('FROM watches WHERE id = ?1'), (w1[0],)).fetchone()
db.execute(q('UPDATE watches'), (w1[0], 1, now, '4 ta poyezd, mos: 1', 1, 'sig', now)); db.execute('COMMIT')
assert db.execute(q('FROM watches WHERE id = ?1'), (w1[0],)).fetchone()[11] == '4 ta poyezd, mos: 1'
# NULLS FIRST: tekshirilmagan (w2) birinchi
assert db.execute(q('WHERE active = 1 ORDER BY last_check NULLS FIRST')).fetchall()[0][0] == w2[0]; print('watches CRUD + navbat OK')

for bad, why in [((u1[0],'1','1','2026-12-05',0.0,0,'',1,now), 'from=to'), ((u1[0],'1','2','05.12.2026',0.0,0,'',1,now), 'sana formati')]:
    try: db.execute(wc, bad); raise SystemExit('CHECK ishlamadi: '+why)
    except sqlite3.IntegrityError: print('CHECK OK:', why)

pg = q('FROM polling_rules ORDER BY max_days_left = 0')
print('polling default:', db.execute(pg).fetchall())
db.execute('BEGIN'); db.execute(q('DELETE FROM polling_rules')); ins = q('INSERT INTO polling_rules (max_days_left, interval_seconds, updated_at')
for d, s in [(0,21600),(3,30)]: db.execute(ins, (d, s, now, 'admin@x.uz'))
db.execute('COMMIT'); print('polling saqlandi:', db.execute(pg).fetchall())
try: db.execute(ins, (5, 5, now, 'x')); raise SystemExit('min 10s CHECK ishlamadi')
except sqlite3.IntegrityError: print('CHECK OK: min 10s')

assert db.execute(q('DELETE FROM watches WHERE id'), (w2[0],)).rowcount == 1
db.execute('DELETE FROM users WHERE id = ?', (u1[0],))
assert db.execute('SELECT count(*) FROM watches').fetchone()[0] == 0; print('ON DELETE CASCADE OK')
print('HAMMASI OK')
