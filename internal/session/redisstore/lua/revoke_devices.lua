-- The existing hash retains its original absolute expiry as a tombstone.
for _,key in ipairs(targets) do
  redis.call('HSET',key,'state','dead')
  redis.call('ZREM',KEYS[2],key)
end
return {1}
