import hashlib, json
from pathlib import Path
import ida_auto, ida_bytes, ida_kernwin, ida_nalt, ida_pro, ida_ua, ida_name, ida_funcs, idautils, idc
ida_auto.auto_wait()
raw=Path(ida_nalt.get_input_file_path()).read_bytes()
assert hashlib.sha256(raw).hexdigest()=="a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38"
rows=[]
for label,start,end in [("LPR",int.from_bytes(raw[0xec+0x9d*2:0xee+0x9d*2],"big"),None),("EQSTR",int.from_bytes(raw[0xec+0xe8*2:0xee+0xe8*2],"big"),None),("讀取錯誤附近",0x3ce,0x416)]:
    at=start
    for i in range(80):
        n=ida_ua.create_insn(at)
        assert n
        line=idc.generate_disasm_line(at,0)
        rows.append(dict(label=label,ida_address=hex(at),bytes=ida_bytes.get_bytes(at,n).hex(),instruction=line,original_name=ida_name.get_name(at),original_function=ida_funcs.get_func_name(at),grade="unknown",semantic=None,xrefs_from=[dict(address=hex(x.to),type=x.type) for x in idautils.XrefsFrom(at)],evidence="IDA9.4／固定直譯器原始bytes"))
        at+=n
        if (end is not None and at>=end) or (end is None and "jmp" in line):break
Path("/out/depot-interp.json").write_text(json.dumps(dict(input_sha256=hashlib.sha256(raw).hexdigest(),version=ida_kernwin.get_kernel_version(),address_space="SYSTEM.INTERP binary base0",rows=rows),ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
ida_pro.qexit(0)
