local code,c = load()
if code ~= 1 then return {code} end
if not matches(c) then return {3} end
if not active(c) then return {2} end
if not indexed(c) then return {3} end
c[10] = string.format('%.0f',math.min(now+idleLimit,tonumber(c[8])))
redis.call('HSET',KEYS[1],'idle',c[10])
redis.call('ZADD',KEYS[2],c[10],KEYS[1])
expireIndex()
return {1,c[10]}
