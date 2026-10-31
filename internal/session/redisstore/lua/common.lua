local parts = redis.call('TIME')
local now = tonumber(parts[1])*1000 + math.floor(tonumber(parts[2])/1000)
local idleLimit, absoluteLimit = tonumber(ARGV[9]), tonumber(ARGV[10])
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
local function load()
  local t = kind(KEYS[1])
  if t == 'none' then return 0 end
  if t ~= 'hash' or redis.call('HLEN',KEYS[1]) ~= #names then return 3 end
  for _,name in ipairs(names) do
    if redis.call('HSTRLEN',KEYS[1],name) > 64 then return 3 end
  end
  local c = redis.call('HMGET',KEYS[1],unpack(names))
  for _,value in ipairs(c) do if type(value) ~= 'string' then return 3 end end
  if c[1] ~= '1' or c[2] ~= ARGV[1] or not hex(c[2],64) or not hex(c[3],32)
    or not version(c[4]) or not version(c[5]) or not hex(c[6],32) or not hex(c[12],64)
    or (c[11] ~= 'live' and c[11] ~= 'dead') then return 3 end
  local created, absolute, initial, idle = stamp(c[7]),stamp(c[8]),stamp(c[9]),stamp(c[10])
  if not created or not absolute or not initial or not idle then return 3 end
  if absolute-created ~= absoluteLimit or initial-created ~= idleLimit or created > now
    or idle < initial or idle > absolute or idle > now+idleLimit then return 3 end
  if absolute <= now then return 2 end
  if redis.call('PEXPIRETIME',KEYS[1]) ~= absolute then return 3 end
  return 1,c
end
local function matches(c)
  return c[3] == ARGV[2] and c[4] == ARGV[3] and c[5] == ARGV[4]
    and c[6] == ARGV[5] and c[7] == ARGV[6] and c[8] == ARGV[7] and c[9] == ARGV[8] and c[12] == ARGV[11]
end
local function indexType()
  local t = kind(KEYS[2])
  return t == 'none' or t == 'zset'
end
local function indexed(c)
  if kind(KEYS[2]) ~= 'zset' then return false end
  local score = tonumber(redis.call('ZSCORE',KEYS[2],KEYS[1]))
  if score ~= tonumber(c[10]) or score <= now or redis.call('ZCARD',KEYS[2]) > 3 then return false end
  local latest = redis.call('ZREVRANGE',KEYS[2],0,0,'WITHSCORES')
  return #latest == 2 and tonumber(latest[2]) <= now+idleLimit
    and redis.call('PEXPIRETIME',KEYS[2]) >= tonumber(latest[2])
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
