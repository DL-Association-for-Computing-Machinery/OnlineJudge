local code,c = load()
if code == 1 then
  if not matches(c) then return {3} end
elseif code == 0 then
  local created,absolute,initial = stamp(ARGV[6]),stamp(ARGV[7]),stamp(ARGV[8])
  if not created or not absolute or not initial or created > now or absolute-created ~= absoluteLimit
    or initial-created ~= idleLimit then return {3} end
  if absolute <= now or initial <= now then return {2} end
  c = {'1',ARGV[1],ARGV[2],ARGV[3],ARGV[4],ARGV[5],ARGV[6],ARGV[7],ARGV[8],ARGV[8],'dead',ARGV[11]}
else return {code} end
if not indexType() then return {3} end
c[11]='dead'
put(c)
redis.call('ZREM',KEYS[2],KEYS[1])
return {1}
