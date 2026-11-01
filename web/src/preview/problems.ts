export type Difficulty = '简单' | '中等'
export interface PreviewProblem {
  id: string
  title: string
  difficulty: Difficulty
  topic: string
  description: string
  input: string
  output: string
  sampleInput: string
  sampleOutput: string
  range: string
}

// These local fixtures are never a fallback for an authorized problem API.
// Their namespace and every entry point explicitly identify them as previews.
export const previewProblems: readonly PreviewProblem[] = [
  { id: 'P1001', title: 'A+B Problem', difficulty: '简单', topic: '输入输出', description: '给定两个整数 a 和 b，输出它们的和。', input: '输入一行，包含两个整数 a 和 b，以空格分隔。', output: '输出一个整数，表示 a 与 b 的和。', sampleInput: '1 2', sampleOutput: '3', range: '−10⁹ ≤ a, b ≤ 10⁹' },
  { id: 'P1002', title: '求和', difficulty: '简单', topic: '循环', description: '计算从 1 到 n 的所有整数之和。', input: '输入一个正整数 n。', output: '输出从 1 到 n 的整数之和。', sampleInput: '5', sampleOutput: '15', range: '1 ≤ n ≤ 10⁶' },
  { id: 'P1003', title: '反转字符串', difficulty: '简单', topic: '字符串', description: '将给定的小写字母字符串逆序输出。', input: '输入一行非空字符串。', output: '输出逆序后的字符串。', sampleInput: 'hello', sampleOutput: 'olleh', range: '字符串长度不超过 1000' },
  { id: 'P1004', title: '括号匹配', difficulty: '中等', topic: '栈', description: '判断只含左右圆括号的字符串是否正确匹配。', input: '输入一行非空括号字符串。', output: '正确匹配输出 YES，否则输出 NO。', sampleInput: '(())()', sampleOutput: 'YES', range: '字符串长度不超过 10⁵' },
  { id: 'P1005', title: '二分查找', difficulty: '简单', topic: '二分', description: '在严格递增的整数序列中查找目标值的位置。', input: '第一行 n 和目标值 x；第二行 n 个严格递增整数。', output: '输出目标值的下标（从 1 开始）；不存在则输出 −1。', sampleInput: '5 7\n1 3 5 7 9', sampleOutput: '4', range: '1 ≤ n ≤ 10⁵' },
  { id: 'P1006', title: '最长递增子序列', difficulty: '中等', topic: '动态规划', description: '求一个整数序列的最长严格递增子序列长度。', input: '第一行序列长度 n；第二行 n 个整数。', output: '输出最长严格递增子序列的长度。', sampleInput: '6\n3 1 2 5 4 6', sampleOutput: '4', range: '1 ≤ n ≤ 10⁵' },
  { id: 'P1007', title: '最大值', difficulty: '简单', topic: '循环', description: '找出给定序列中的最大整数。', input: '第一行 n；第二行 n 个整数。', output: '输出序列最大值。', sampleInput: '4\n2 8 3 1', sampleOutput: '8', range: '1 ≤ n ≤ 1000' },
  { id: 'P1008', title: '奇偶判断', difficulty: '简单', topic: '输入输出', description: '判断给定整数是否为偶数。', input: '输入一个整数 n。', output: '偶数输出 EVEN，奇数输出 ODD。', sampleInput: '8', sampleOutput: 'EVEN', range: '−10⁹ ≤ n ≤ 10⁹' },
  { id: 'P1009', title: '数组去重', difficulty: '简单', topic: '排序', description: '将序列中的不同整数按从小到大顺序输出。', input: '第一行 n；第二行 n 个整数。', output: '输出排序去重后的整数，以一个空格分隔。', sampleInput: '5\n3 1 3 2 1', sampleOutput: '1 2 3', range: '1 ≤ n ≤ 10⁵' },
  { id: 'P1010', title: '最短路径', difficulty: '中等', topic: '图', description: '求无权无向连通图中从节点 1 到节点 n 的最少边数。', input: '第一行 n、m，随后 m 行给出每条边的两个端点。', output: '输出从节点 1 到节点 n 的最少边数。', sampleInput: '3 2\n1 2\n2 3', sampleOutput: '2', range: '2 ≤ n ≤ 10⁵，n−1 ≤ m ≤ 2×10⁵' },
  { id: 'P1011', title: '连通分量', difficulty: '中等', topic: '图', description: '统计无向图的连通分量数量。', input: '第一行 n、m，随后 m 行给出每条边的两个端点。', output: '输出连通分量的数量。', sampleInput: '4 2\n1 2\n3 4', sampleOutput: '2', range: '1 ≤ n ≤ 10⁵，0 ≤ m ≤ 2×10⁵' },
  { id: 'P1012', title: '合并有序序列', difficulty: '简单', topic: '排序', description: '将两个递增整数序列合并为一个非递减序列。', input: '第一行 n、m；第二行 n 个整数；第三行 m 个整数。', output: '输出合并后的序列，以一个空格分隔。', sampleInput: '2 3\n1 4\n2 3 5', sampleOutput: '1 2 3 4 5', range: '1 ≤ n, m ≤ 10⁵' },
]

export function selectPreviewProblems(query: string, difficulty: string, topic: string) {
  const needle = query.slice(0, 80).toLocaleLowerCase()
  return previewProblems.filter(problem =>
    `${problem.id} ${problem.title}`.toLocaleLowerCase().includes(needle) &&
    (!difficulty || problem.difficulty === difficulty) && (!topic || problem.topic === topic))
}
