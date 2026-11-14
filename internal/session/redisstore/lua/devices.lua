local code,c = load()
if code ~= 1 then return {code} end
if not matches(c) then return {3} end
if not active(c) then return {2} end
if not indexed(c) then return {3} end
local prefix = string.sub(KEYS[1],1,#KEYS[1]-64)
local members = redis.call('ZRANGE',KEYS[2],0,-1,'WITHSCORES')
local records = {}
for i=1,#members,2 do
  local key,score = members[i],tonumber(members[i+1])
  if score > now then
    local d = string.sub(key,#prefix+1)
    if string.sub(key,1,#prefix) ~= prefix or not hex(d,64) then return {3} end
    local found,record = loadRecord(key,d)
    if found ~= 1 or not active(record) or tonumber(record[10]) ~= score or
      record[3] ~= ARGV[2] or record[5] ~= ARGV[4] or record[6] ~= ARGV[5] then return {3} end
    records[#records+1] = record
  end
end
