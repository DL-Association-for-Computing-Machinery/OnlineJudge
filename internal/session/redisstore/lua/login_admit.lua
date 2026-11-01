#!lua flags=no-cluster
-- session-protocol:login-admit
-- Validate ALL counters before deciding quota or writing anything. Rejection
-- never creates high-cardinality keys or extends a successful request's TTL.
local function integer(s, minimum, maximum)
    if type(s) ~= 'string' or #s > 16 or not string.match(s, '^%d+$') then return nil end
    local n = tonumber(s)
    if not n or n < minimum or n > maximum or string.format('%.0f', n) ~= s then return nil end
    return n
end
local safe = 9007199254740991
if (#KEYS ~= 3 and #KEYS ~= 4) or #ARGV ~= 1 + 2 * (#KEYS - 1) then return -1 end
if redis.call('TYPE', KEYS[1]).ok ~= 'string' or
   redis.call('GET', KEYS[1]) ~= ARGV[1] or
   redis.call('PEXPIRETIME', KEYS[1]) ~= -1 then return -1 end
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
if now < 0 or now > safe - 86400000 then return -1 end
local counts, starts, deadlines = {}, {}, {}
local denied = false
for i = 2, #KEYS do
    local limit = integer(ARGV[2 * i - 2], 1, 1000000)
    local window = integer(ARGV[2 * i - 1], 1, 86400000)
    if not limit or not window then return -1 end
    local start = math.floor(now / window) * window
    local deadline = start + window
    local count = 0
    local kind = redis.call('TYPE', KEYS[i]).ok
    if kind ~= 'none' then
        if kind ~= 'string' then return -1 end
        local raw = redis.call('GET', KEYS[i])
        if not raw or #raw > 48 then return -1 end
        local s, c, d = string.match(raw, '^(%d+):(%d+):(%d+)$')
        s, c, d = integer(s, 0, safe), integer(c, 1, limit), integer(d, 1, safe)
        if not s or not c or not d or s % window ~= 0 or s > safe - window or
           d ~= s + window or redis.call('PEXPIRETIME', KEYS[i]) ~= d then return -1 end
        -- At equality Redis can still physically retain the old key. Treat a
        -- completely validated expired window as empty, without an early write.
        if now < d then
            if s ~= start then return -1 end
            count = c
        end
    end
    if count >= limit then denied = true end
    counts[i], starts[i], deadlines[i] = count, start, deadline
end
if denied then return 0 end
-- Global first bounds key creation even if a later SET fails. Each individual
-- SET includes PXAT, so partial execution cannot leave immortal counter keys.
for i = 2, #KEYS do
    local raw = string.format('%.0f:%.0f:%.0f', starts[i], counts[i] + 1, deadlines[i])
    redis.call('SET', KEYS[i], raw, 'PXAT', string.format('%.0f', deadlines[i]))
end
return 1
