-- Core/Journal.lua: sessioni ed eventi (DiarioGildaDB), singleton a livello di modulo.
--   DG.Journal.load(saved, deps) -> db   deps = { now(), where() -> {z,s,m,x,y,inst} }; db torna in DiarioGildaDB
--   DG.Journal.begin(info, isReload)     info = {char, realm, guild, class, race, level, build}; apre o riapre
--   DG.Journal.add(k, fields) -> event   nil senza sessione aperta o oltre MAX_EVENTS (in tal caso conta in dropped)
--   DG.Journal.finish()                  chiude la sessione corrente (stop = now)
--   DG.Journal.firstVisit(zone) -> bool  vero solo la prima volta per il personaggio corrente
--   DG.Journal.current() -> session|nil  sessione corrente
--   DG.Journal.setGuild(name)            gilda letta in ritardo: la scrive solo se la sessione aperta non ce l'ha
--   DG.Journal.logError(msg)             tiene gli ultimi MAX_ERRORS errori
local _, DG = ...

local Journal = {}
DG.Journal = Journal

local db = nil
local deps = nil
local cur = nil

local function key(s)
  return s.realm .. "-" .. s.char
end

local function lastEventTime(s)
  local last = s.events[#s.events]
  return last and last.t or s.start
end

function Journal.load(saved, d)
  deps = d
  cur = nil
  db = type(saved) == "table" and saved or {}
  db.v = 1
  db.sessions = db.sessions or {}
  db.seen = db.seen or {}
  db.errors = db.errors or {}
  return db
end

function Journal.begin(info, isReload)
  local now = deps.now()

  -- pota le sessioni troppo vecchie
  local sessions = {}
  for _, s in ipairs(db.sessions) do
    if s.start >= now - DG.Const.KEEP_SECONDS then
      sessions[#sessions + 1] = s
    end
  end
  db.sessions = sessions

  local k = info.realm .. "-" .. info.char

  -- /reload: riapre l'ultima sessione dello stesso personaggio (aperta o chiusa da poco)
  if isReload then
    for i = #sessions, 1, -1 do
      local s = sessions[i]
      if key(s) == k then
        if s.stop == nil or now - s.stop <= DG.Const.RELOAD_GAP then
          s.stop = nil
          cur = s
          return s
        end
        break
      end
    end
  end

  -- login normale: ogni sessione ancora aperta del personaggio e' un crash, la chiude all'ultimo evento
  for _, s in ipairs(sessions) do
    if s.stop == nil and key(s) == k then
      s.stop = lastEventTime(s)
    end
  end

  cur = {
    id = k .. "-" .. now,
    char = info.char, realm = info.realm, guild = info.guild,
    class = info.class, race = info.race, level = info.level,
    build = info.build, addon = DG.Const.VERSION,
    start = now, dropped = 0, events = {},
  }
  sessions[#sessions + 1] = cur
  return cur
end

function Journal.add(k, fields)
  if not cur or cur.stop ~= nil then
    return nil
  end
  if #cur.events >= DG.Const.MAX_EVENTS then
    cur.dropped = cur.dropped + 1
    return nil
  end
  local ev = {}
  if fields then
    for f, v in pairs(fields) do
      ev[f] = v
    end
  end
  ev.t = deps.now()
  ev.k = k
  local ok, w = pcall(deps.where)
  if ok and type(w) == "table" then
    for f, v in pairs(w) do
      if ev[f] == nil then
        ev[f] = v
      end
    end
  end
  cur.events[#cur.events + 1] = ev
  return ev
end

function Journal.finish()
  if cur then
    cur.stop = deps.now()
  end
end

function Journal.firstVisit(zone)
  if not cur or zone == nil then
    return false
  end
  local k = key(cur)
  db.seen[k] = db.seen[k] or {}
  if db.seen[k][zone] then
    return false
  end
  db.seen[k][zone] = true
  return true
end

function Journal.setGuild(name)
  if cur and cur.stop == nil and cur.guild == nil and type(name) == "string" then
    cur.guild = name
  end
end

function Journal.current()
  return cur
end

function Journal.logError(msg)
  local errors = db.errors
  errors[#errors + 1] = tostring(msg)
  while #errors > DG.Const.MAX_ERRORS do
    table.remove(errors, 1)
  end
end
