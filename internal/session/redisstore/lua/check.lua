local code,c = load()
if code == 0 and (tonumber(ARGV[7]) <= now or tonumber(ARGV[8]) <= now) then return {2} end
if code ~= 1 then return {code} end
if not matches(c) then return {3} end
if not active(c) then return {2} end
if c[10] ~= c[9] or not indexed(c) then return {3} end
return {1}
