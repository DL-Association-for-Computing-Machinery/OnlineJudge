if #targets ~= 1 then return {3} end
local c = {'1',ARGV[1],ARGV[2],ARGV[3],ARGV[4],ARGV[5],ARGV[6],ARGV[7],ARGV[8],ARGV[8],'live',ARGV[11]}
-- The candidate is not usable until indexed and independently checked.
put(c)
redis.call('HSET',targets[1],'state','dead')
redis.call('ZREM',KEYS[2],targets[1])
redis.call('ZADD',KEYS[2],c[10],KEYS[1])
expireIndex()
return {1}
