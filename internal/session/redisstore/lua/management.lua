local key, op, expected = KEYS[1], ARGV[1], ARGV[2]
local idleWindow, lifetime, maxRecord = tonumber(ARGV[3]), tonumber(ARGV[4]), tonumber(ARGV[5])
local parts = redis.call('TIME')
local now = tonumber(parts[1])*1000 + math.floor(tonumber(parts[2])/1000)
local function stamp(value)
  if not value or #value > 16 or not string.match(value,'^[1-9][0-9]*$') then return nil end
  local n = tonumber(value)
  if n > 9007199254740991 then return nil end
  return n
end
local function pack(prefix,created,absolute,idle)
  return prefix..string.format('%.0f:%.0f:%.0f',created,absolute,idle)
end
if op == 'issue' then
  local raw = pack(expected,now,now+lifetime,now+idleWindow)
  if #raw > maxRecord then return {3} end
  -- This operation is never replayed with the same token. No touch can recreate it.
  if not redis.call('SET',key,raw,'NX','PXAT',now+idleWindow) then return {2} end
  return {1,raw}
end
local kind = redis.call('TYPE',key).ok
if kind == 'none' then return {0} end
if kind ~= 'string' or redis.call('STRLEN',key) > maxRecord then return {3} end
local raw = redis.call('GET',key)
if not raw then return {0} end
local prefix,c,a,i = string.match(raw,'^(.*:)([0-9]+):([0-9]+):([0-9]+)$')
local created,absolute,idle = stamp(c),stamp(a),stamp(i)
if not created or not absolute or not idle or created > now or absolute-created ~= lifetime
  or idle < created+idleWindow or idle > absolute or idle > now+idleWindow
  or redis.call('PEXPIRETIME',key) ~= idle then return {3} end
if idle <= now or absolute <= now then return {0} end
if op == 'locate' then return {1,raw} end
if raw ~= expected then return {2} end
if op == 'read' then return {1,raw} end
if op == 'revoke' then
  if redis.call('DEL',key) ~= 1 then return {0} end
  return {1}
end
if op == 'touch' then
  local nextIdle = math.min(now+idleWindow,absolute)
  local next = pack(prefix,created,absolute,nextIdle)
  if not redis.call('SET',key,next,'XX','PXAT',nextIdle) then return {0} end
  return {1,next}
end
return {3}
