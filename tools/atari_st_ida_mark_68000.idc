// 針對 Atari ST raw TOS binary 的候選正文建立可回查函式邊界。
// 不改名、不改原始 bytes；這只是獨立 IDA evidence database 的導覽標記。
#include <idc.idc>

static main()
{
  auto start = 0x288;
  auto end = 0x3e0;

  // 這段候選正文由 raw bytes 與後續 68000 控制流交叉追查而來；先解除
  // Binary loader 的資料標記，再讓 IDA 依原始指令建立函式與 xref。
  MakeUnkn(start, end - start);
  MakeCode(start);
  MakeFunction(start, end);
  AutoMark(start, AU_CODE);
  auto_wait();

  // 與 IDA 內建 analysis.idc 相同的輸出方式；反組譯仍只寫入 /tmp 的
  // 獨立資料庫，不會改寫 repository 或原始磁碟映像。
  auto asm_path = get_idb_path()[0:-4] + ".asm";
  gen_file(OFILE_ASM, fopen(asm_path, "w"), 0, BADADDR, 0);
  qexit(0);
}
