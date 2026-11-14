local code,c = load()
if code ~= 1 then return {code} end
if not matches(c) then return {3} end
if not active(c) then return {2} end
if not indexed(c) then return {3} end
local d,binding = ARGV[13],ARGV[14]
local prefix = string.sub(KEYS[1],1,#KEYS[1]-64)
if not hex(d,64) or not hex(binding,64) or KEYS[3] ~= prefix..d
  or kind(KEYS[3]) ~= 'none' or redis.call('ZSCORE',KEYS[2],KEYS[3]) then return {3} end
-- Stage an unindexed successor, invalidate the original, then move its slot.
-- Keeping the same score preserves both idle time and index expiry.
c[2],c[12] = d,binding
local fields = {}
for i,name in ipairs(names) do
  fields[#fields+1]=name; fields[#fields+1]=c[i]
end
local indexExpiry = redis.call('PEXPIRETIME',KEYS[2])
redis.call('HSET',KEYS[3],unpack(fields))
redis.call('PEXPIREAT',KEYS[3],c[8])
redis.call('HSET',KEYS[1],'state','dead')
redis.call('ZREM',KEYS[2],KEYS[1])
redis.call('ZADD',KEYS[2],c[10],KEYS[3])
redis.call('PEXPIREAT',KEYS[2],string.format('%.0f',indexExpiry))
return {1}
