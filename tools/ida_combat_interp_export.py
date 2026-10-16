"""IDA 9.4 直譯器測試配置匯出；入口見 talos-combat-audit-20260907.md。"""
import json, hashlib
from pathlib import Path
import ida_auto, ida_bytes, ida_kernwin, ida_nalt, ida_pro, ida_ua, ida_name, ida_funcs, idautils, idc
ida_auto.auto_wait()
p=Path(ida_nalt.get_input_file_path())
raw=p.read_bytes()
assert hashlib.sha256(raw).hexdigest()=="a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38"
rows=[]
for opcode in [0x3c,0x29,0x68,0xc5,0xeb,0x82,0xca,0xe5,0xa1,0xad,0xa6,0x9a,0x9b,0xd9,0x73,0x91,0x96,0xd5,0x8b,0xef,0xd6,0xd3,0xc9,0xd2,0xf1]:
    entry=int.from_bytes(raw[0xec+opcode*2:0xee+opcode*2],"big")
    at=entry
    for i in range(60):
        n=ida_ua.create_insn(at)
        if not n: break
        line=idc.generate_disasm_line(at,0)
        rows.append(dict(opcode=hex(opcode),ida_address=hex(at),bytes=ida_bytes.get_bytes(at,n).hex(),instruction=line,grade="unknown",warning="僅供直譯器測試記憶體配置；非玩法語意",evidence="固定來源雜湊及 IDA 9.4"))
        at+=n
        if "jmp" in line or line.strip().startswith("bra"): break
at=0x45e
for label, first in [("mov-block-tail",0x930)]:
    at=first
    for i in range(60):
        n=ida_ua.create_insn(at)
        assert n
        line=idc.generate_disasm_line(at,0)
        rows.append(dict(opcode=label,ida_address=hex(at),bytes=ida_bytes.get_bytes(at,n).hex(),instruction=line,grade="unknown",warning="僅供研究框架的快取還原配置；非玩法語意",evidence="固定來源雜湊及 IDA 9.4"))
        at+=n
        if "jmp" in line: break
at=0x45e
while at<0x476:
    n=ida_ua.create_insn(at)
    assert n
    rows.append(dict(opcode="static-link-helper",ida_address=hex(at),bytes=ida_bytes.get_bytes(at,n).hex(),instruction=idc.generate_disasm_line(at,0),grade="unknown",warning="僅供測試記憶體配置",evidence="固定來源雜湊及 IDA 9.4"))
    at+=n
at=0x668
while at<0x690:
    n=ida_ua.create_insn(at)
    assert n
    rows.append(dict(opcode="external-address-helper",ida_address=hex(at),bytes=ida_bytes.get_bytes(at,n).hex(),instruction=idc.generate_disasm_line(at,0),grade="unknown",warning="僅供測試記憶體配置",evidence="固定來源雜湊及 IDA 9.4"))
    at+=n
for label, first, last in [("call-frame-helper",0xda2,0xe22),("external-call-helper",0xe9e,0xed4),("case-word-helper",0xd58,0xd64)]:
    at=first
    while at<last:
        n=ida_ua.create_insn(at)
        assert n and at+n<=last
        rows.append(dict(opcode=label,ida_address=hex(at),bytes=ida_bytes.get_bytes(at,n).hex(),instruction=idc.generate_disasm_line(at,0),grade="unknown",warning="僅供受控測試的呼叫參數配置；非玩法語意",evidence="固定來源雜湊及 IDA 9.4"))
        at+=n
for row in rows:
    address=int(row["ida_address"],16)
    row.update(original_name=ida_name.get_name(address),original_function=ida_funcs.get_func_name(address),semantic=None,xrefs_from=[{"address":hex(x.to),"type":x.type} for x in idautils.XrefsFrom(address)])
Path("/out/interp-probe.json").write_text(json.dumps(dict(input_sha256=hashlib.sha256(raw).hexdigest(),version=ida_kernwin.get_kernel_version(),address_space="SYSTEM.INTERP binary base 0",rows=rows),ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
ida_pro.qexit(0)
