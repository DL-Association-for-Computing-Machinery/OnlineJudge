-- Go checked each immutable binding. Confirm the complete active set is still
-- the one it checked before writing; renewal may change idle time and order.
if #ARGV ~= 12 + 3 * #records then return {3} end
local expected = {}
for i=13,#ARGV,3 do
  local d, invariant, revoke = ARGV[i], ARGV[i+1], ARGV[i+2]
  if expected[d] or (revoke ~= '0' and revoke ~= '1') or
    (d == ARGV[1] and revoke == '1') then return {3} end
  expected[d] = {invariant,revoke}
end
local targets = {}
for _,record in ipairs(records) do
  local entry = expected[record[2]]
  local invariant = table.concat({record[4],record[7],record[8],record[9],record[12]},':')
  if not entry or entry[1] ~= invariant then return {3} end
  if entry[2] == '1' then targets[#targets+1] = prefix..record[2] end
end
