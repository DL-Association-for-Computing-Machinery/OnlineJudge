local code,c = load()
if code ~= 1 then return {code} end
if not matches(c) then return {3} end
if not active(c) then return {2} end
if not indexed(c) then return {3} end
return {1,c[10]}
