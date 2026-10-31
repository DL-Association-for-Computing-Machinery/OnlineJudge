local code,c = load()
if code ~= 1 then return {code} end
return {1,unpack(c)}
