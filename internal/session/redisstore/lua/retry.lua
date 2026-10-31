local code,c = load()
if code == 1 then
  if not matches(c) then return {3} end
  if not active(c) then return {2} end
  if c[10] ~= c[9] or not indexed(c) then return {3} end
  return {1}
end
if code ~= 0 then return {code} end
local created,absolute,initial = stamp(ARGV[6]),stamp(ARGV[7]),stamp(ARGV[8])
if not created or not absolute or not initial or created > now or absolute-created ~= absoluteLimit
  or initial-created ~= idleLimit then return {3} end
if absolute <= now or initial <= now then return {2} end
if not indexType() then return {3} end
if kind(KEYS[2]) == 'zset' then
  if redis.call('ZCARD',KEYS[2]) > 3 then return {3} end
  local latest = redis.call('ZREVRANGE',KEYS[2],0,0,'WITHSCORES')
  if #latest == 2 and tonumber(latest[2]) > now+idleLimit then return {3} end
end
redis.call('ZREMRANGEBYSCORE',KEYS[2],'-inf',now)
if redis.call('ZCARD',KEYS[2]) >= 3 then return {4} end
c = {'1',ARGV[1],ARGV[2],ARGV[3],ARGV[4],ARGV[5],ARGV[6],ARGV[7],ARGV[8],ARGV[8],'live',ARGV[11]}
put(c)
redis.call('ZADD',KEYS[2],c[10],KEYS[1])
expireIndex()
return {1}
