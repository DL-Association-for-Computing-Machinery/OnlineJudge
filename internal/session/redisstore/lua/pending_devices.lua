if not loginOpen then return {2} end
local code = load()
if code ~= 0 then
  if code == 1 then return {3} end
  return {code}
end
local created,absolute,initial = stamp(ARGV[6]),stamp(ARGV[7]),stamp(ARGV[8])
if not validProfile(created,absolute,initial) then return {3} end
if absolute <= now or initial <= now then return {2} end
if not indexType() or redis.call('ZCARD',KEYS[2]) > 3 then return {3} end
