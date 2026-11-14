from pathlib import Path
import hashlib,json,sys
root=Path(sys.argv[1]); root.mkdir(mode=0o700)
b=root/'bundle'; b.mkdir(mode=0o700)
def canon(x): return (json.dumps(x,ensure_ascii=False,separators=(',',':'),sort_keys=True)+'\n').encode()
def ident(x): return dict(sha256=hashlib.sha256(x).hexdigest(),size=len(x))
files={}
def file(n,x):
 if isinstance(x,str):x=x.encode()
 files[n]=x
 p=b/n;p.parent.mkdir(mode=0o700,parents=True,exist_ok=True);p.write_bytes(x);p.chmod(0o600)
 return dict(ident(x),path=n)
def obj(x):
 if isinstance(x,str):x=x.encode()
 return file('objects/sha256/'+ident(x)['sha256'],x)
s='local/bounded_span'; sid='library-checker:bounded_span'; title='数列极差'
spj = sys.argv[2:] == ['--spj']
decimal = sys.argv[2:] == ['--decimal']
if spj:
 title = '数列极差（SPJ）'
if decimal:
 s='local/decimal_span'; sid='library-checker:decimal_span'; title='十分之一极差（浮点）'
source={}
def src(name,x): source[name]=obj(x)
if spj:
 src('quality/checker.cpp', (Path(__file__).resolve().parents[2] / 'sandbox/testdata/testlib_span_checker.cpp').read_bytes())
base='#include <iostream>\n#include <algorithm>\n#include <vector>\nusing namespace std;\n'
if decimal:
 src('quality/decimal.json',canon(dict(absolute='0.01',relative='0.001',text=['RANGE'])))
 base+='#include <iomanip>\nvoid output(int x){cout<<"RANGE "<<fixed<<setprecision(1)<<x/10.0<<"\\n";}\n'
def print_value(expr):
 return f'output({expr});' if decimal else f'cout<<{expr}<<"\\n";'
src('sol/correct.cpp',base+'int main(){int n,x;cin>>n>>x;int a=x,b=x;while(--n){cin>>x;a=min(a,x);b=max(b,x);}'+print_value('b-a')+'}\n')
src('quality/validator.cpp',base+'int main(){int n,x;if(!(cin>>n)||n<1||n>64)return 1;while(n--){if(!(cin>>x)||x < -1000 || x>1000)return 1;}string extra;return cin>>extra?1:0;}\n')
src('quality/oracle.cpp',base+'int main(){int n;cin>>n;vector<int>a(n);for(int& x:a)cin>>x;int best=0;for(int x:a)for(int y:a)best=max(best,x-y);'+print_value('best')+'}\n')
src('quality/coverage.cpp',base+'int main(){int n;cin>>n;vector<int>a(n);for(int& x:a)cin>>x;int lo=*min_element(a.begin(),a.end()),hi=*max_element(a.begin(),a.end());if(n==1)cout<<"singleton ";if(n==64)cout<<"max-size ";if(lo==-1000)cout<<"lower-bound ";if(hi==1000)cout<<"upper-bound ";if(hi<0)cout<<"all-negative ";if(hi==lo&&n>1)cout<<"equal ";if(lo<0&&hi>0)cout<<"mixed ";if(lo!=a.front()&&hi!=a.back())cout<<"internal-extrema ";}\n')
wrong={'max-only':print_value('b'),'endpoints':print_value('abs(a.front()-a.back())'),'zero-floor':print_value('b-min(l,0)'),'distinct-count':'sort(a.begin(),a.end());'+print_value('int(unique(a.begin(),a.end())-a.begin())-1')}
for k,expr in wrong.items():src('quality/wrong/'+k+'.cpp',base+'int main(){int n;cin>>n;vector<int>a(n);for(int& x:a)cin>>x;int l=*min_element(a.begin(),a.end()),b=*max_element(a.begin(),a.end());'+expr+'}\n')
def data(a):return str(len(a))+'\n'+' '.join(map(str,a))+'\n'
probes=[[0],[42],[-1],[0,4],[4,0],[-5,-7],[7,1,8,2],[-8,0,7,0],[4,4,4],[-6,8,8,-6],[8,7,6,5],[-1000,1000]]
for i,a in enumerate(probes):src(f'quality/inputs/probe-{i}.in',data(a))
for k,x in {'good':'2\n1 2\n','short':'3\n1 2\n','outside':'1\n1001\n','extra':'1\n0 5\n'}.items():src('quality/inputs/'+k+'.in',x)
config=dict(controls=[dict(input=x,accept=x=='good') for x in ('good','short','outside','extra')],probes=[f'probe-{i}' for i in range(len(probes))],
 coverage={'singleton':[2],'max-size':[10],'lower-bound':[7],'upper-bound':[8],'all-negative':[4],'equal':[6],'mixed':[9],'internal-extrema':[5]},wrong=list(wrong))
# Go typed JSON field order and lexically ordered named coverage.
config['coverage']=dict(sorted(config['coverage'].items()))
src('quality/plan.json',(json.dumps(config,separators=(',',':'))+'\n').encode())
cases=[[2,-3,4],[-4],[1,7,2,3],[-4,-2,-9],[5,100,-70,4],[6,6,6],[-1000,-3,-2],[999,1000,2],[-30,40,0],[x%17-8 for x in range(64)],[4,3],[-2,5,5,-1],[0,0,1]]
tests=[]; mapping=[]; hashes={}
for i,a in enumerate(cases):
 n='example_00' if i==0 else f'edge_{i-1:02d}'
 inp=obj(data(a)); value=max(a)-min(a)
 ans=obj(f'RANGE {value//10}.{value%10}\n' if decimal else str(value)+'\n')
 hashes[n+'.in']=inp['sha256'];hashes[n+'.out']=ans['sha256']
 id=f'case-{i+1:04d}';tests.append(dict(id=id,input=inp,answer=ans,sample=i==0));mapping.append(dict(case_id=id,upstream_case=n))
src('info.toml',f'[[tests]]\nname="example.in"\nnumber=1\n[[tests]]\nname="edge.cpp"\nnumber={len(cases)-1}\n')
src('hash.json',canon(hashes))
license=obj('Synthetic local acceptance fixture, CC0.\n')
repo='https://github.com/yosupo06/library-checker-problems'
commit='1111111111111111111111111111111111111111'
original=dict({'source/LICENSE':license},**{'source/'+s+'/'+n:r for n,r in source.items()})
lock=dict(schema='oj.library-checker-source.v1',repository=repo,commit=commit,problems=[dict(path=s,title_zh=title)],files={n.removeprefix('source/'):{k:v for k,v in r.items() if k!='path'} for n,r in original.items()})
raw=canon(lock); (root/'library-checker.json').write_bytes(raw); file('provenance/source-lock.json',raw)
for n in ('source-catalog','source-validation','material-map'):file('provenance/'+n+'.json','{}\n')
statement=file('statements/span.md','# Range of a sequence\n\nGiven $N$ integers, print the largest minus the smallest.\n\n## Constraints\n\n- $1 \\leq N \\leq 64$\n- $-1000 \\leq a_i \\leq 1000$\n\n## Input\n\n```text\nN\na_1 ... a_N\n```\n\n## Output\n\nPrint one integer. In the sample, the largest is $4$ and the smallest is $-3$, so the answer is $7$.\n')
if spj:
 statement=file('statements/span.md', files['statements/span.md'].decode().replace('Print one integer.', 'Print the range as one integer, or print two integers in $[-2000, 2000]$ whose sum equals the range.'))
if decimal:
 statement=file('statements/span.md', files['statements/span.md'].decode().replace('print the largest minus the smallest.', 'divide the range by 10.').replace('Print one integer.', 'Print RANGE followed by the range divided by 10. An absolute error of at most 0.01 or a relative error of at most 0.001 is accepted.').replace('the answer is $7$', 'print RANGE 0.7'))
statement['language']='en'
doc=dict(schema='oj.problem-draft.v1',adapter='library-checker-pinned-v1',state='private-draft',title=title,statement=statement,test_cases=tests,examples=['case-0001'],judging=dict(mode='tokens',case_sensitive=True,numeric_equivalence=False,token_comparison='exact-bytes',compatibility='configured-not-validated',limits=None,production_profile=None),
 provenance=dict(commit=commit,repository=repo,original_files=original,original_title='Range of a sequence',source_id=sid,source_lock_sha256=ident(raw)['sha256'],source_path=s,upstream_time_limit_seconds=2.0,source_case_mapping=mapping),
 validation=dict(status='not-run',publication_blockers=['statement-rendering-and-formula-validation','production-checker-compatibility-and-resource-admission','independent-problem-validation','authorized-non-author-review','authorized-atomic-publication']))
doc['version']=ident(canon(doc))['sha256']
file('drafts/span.json',canon(doc))
idx=dict(schema='oj.private-draft-bundle.v1',state='private-unpublished',access='synthetic isolated acceptance fixture',copied_source_bytes=sum(map(len,files.values())),files={n:ident(x) for n,x in files.items()},problems=[dict(draft='drafts/span.json',source_id=sid,version=doc['version'],test_cases=len(tests),examples=1)])
file('index.json',canon(idx))
