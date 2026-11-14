local parts = redis.call('TIME')
local now = tonumber(parts[1])*1000 + math.floor(tonumber(parts[2])/1000)
local idleLimit, absoluteLimit = tonumber(ARGV[9]), tonumber(ARGV[10])
local rememberedIdle, rememberedAbsolute = 86400000, 604800000
local function validProfile(created, absolute, initial)
  if not created or not absolute or not initial or created > now then return false end
  local idle, lifetime = initial-created, absolute-created
  return (idle == idleLimit and lifetime == absoluteLimit)
    or (idle == rememberedIdle and lifetime == rememberedAbsolute)
end
local names = {'v','token','subject','credential','auth','recovery','created','absolute','initial','idle','state','binding'}
local function kind(key) return redis.call('TYPE',key).ok end
local function hex(value,n) return #value == n and string.match(value,'^[0-9a-f]+$') ~= nil end
local function version(value)
  return string.match(value,'^[1-9][0-9]*$') ~= nil and
    (#value < 19 or (#value == 19 and value <= '9223372036854775807'))
end
local function stamp(value)
  if not version(value) then return nil end
  local n = tonumber(value)
  if n > 9007199254740991 or math.floor(n) ~= n then return nil end
  return n
end
local function loadRecord(key, digest)
  local t = kind(key)
  if t == 'none' then return 0 end
  if t ~= 'hash' or redis.call('HLEN',key) ~= #names then return 3 end
  for _,name in ipairs(names) do
    if redis.call('HSTRLEN',key,name) > 64 then return 3 end
  end
  local c = redis.call('HMGET',key,unpack(names))
  for _,value in ipairs(c) do if type(value) ~= 'string' then return 3 end end
  if c[1] ~= '1' or c[2] ~= digest or not hex(c[2],64) or not hex(c[3],32)
    or not version(c[4]) or not version(c[5]) or not hex(c[6],32) or not hex(c[12],64)
    or (c[11] ~= 'live' and c[11] ~= 'dead') then return 3 end
  local created, absolute, initial, idle = stamp(c[7]),stamp(c[8]),stamp(c[9]),stamp(c[10])
  if not created or not absolute or not initial or not idle then return 3 end
  if not validProfile(created,absolute,initial)
    or idle < initial or idle > absolute or idle > now+(initial-created) then return 3 end
  if absolute <= now then return 2 end
  if redis.call('PEXPIRETIME',key) ~= absolute then return 3 end
  return 1,c
end
local function load() return loadRecord(KEYS[1],ARGV[1]) end
local function matches(c)
  return c[3] == ARGV[2] and c[4] == ARGV[3] and c[5] == ARGV[4]
    and c[6] == ARGV[5] and c[7] == ARGV[6] and c[8] == ARGV[7] and c[9] == ARGV[8] and c[12] == ARGV[11]
end
local function indexType()
  local t = kind(KEYS[2])
  return t == 'none' or t == 'zset'
end
local function latestIndexValid(latest)
  if #latest ~= 2 then return false end
  local key = latest[1]
  local prefix = string.sub(KEYS[1],1,#KEYS[1]-64)
  local d = string.sub(key,#prefix+1)
  if string.sub(key,1,#prefix) ~= prefix or not hex(d,64) then return false end
  local code,record = loadRecord(key,d)
  return code == 1 and record[11] == 'live' and record[3] == ARGV[2] and record[5] == ARGV[4]
    and record[6] == ARGV[5] and tonumber(latest[2]) == tonumber(record[10])
    and redis.call('PEXPIRETIME',KEYS[2]) >= tonumber(latest[2])
end
local function indexed(c)
  if kind(KEYS[2]) ~= 'zset' then return false end
  local score = tonumber(redis.call('ZSCORE',KEYS[2],KEYS[1]))
  if score ~= tonumber(c[10]) or score <= now or redis.call('ZCARD',KEYS[2]) > 3 then return false end
  local latest = redis.call('ZREVRANGE',KEYS[2],0,0,'WITHSCORES')
  return latestIndexValid(latest)
end
local function active(c) return c[11] == 'live' and tonumber(c[10]) > now end
local function put(c)
  local args = {}
  for i,name in ipairs(names) do args[#args+1]=name; args[#args+1]=c[i] end
  redis.call('HSET',KEYS[1],unpack(args))
  redis.call('PEXPIREAT',KEYS[1],c[8])
end
local function expireIndex()
  local latest = redis.call('ZREVRANGE',KEYS[2],0,0,'WITHSCORES')
  if #latest == 2 then redis.call('PEXPIREAT',KEYS[2],latest[2]) end
end
-- The login envelope authenticates this optional bound. Keep ordinary stored
-- records unchanged so locating/reading an admitted session needs no envelope.
local loginUntil = 0
if ARGV[12] ~= '0' then
  loginUntil = stamp(ARGV[12] or '')
  local created,initial = stamp(ARGV[6]),stamp(ARGV[8])
  if not loginUntil or not created or not initial or loginUntil <= created
    or loginUntil > initial or loginUntil-created > 300000 then return {3} end
end
local loginOpen = loginUntil == 0 or now < loginUntil
