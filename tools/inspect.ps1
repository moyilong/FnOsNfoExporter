param([string]$Db,[string]$Query)
Add-Type @'
using System; using System.Runtime.InteropServices;
public class NativeSqlite {
 [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] public static extern int sqlite3_open_v2(byte[] f,out IntPtr db,int flags,IntPtr vfs);
 [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] public static extern int sqlite3_prepare_v2(IntPtr db,byte[] sql,int n,out IntPtr stmt,IntPtr tail);
 [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] public static extern int sqlite3_step(IntPtr s);
 [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] public static extern int sqlite3_column_count(IntPtr s);
 [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] public static extern IntPtr sqlite3_column_text(IntPtr s,int i);
 [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] public static extern int sqlite3_column_bytes(IntPtr s,int i);
 [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] public static extern int sqlite3_finalize(IntPtr s);
 [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] public static extern int sqlite3_close(IntPtr s);
 public static string Read(IntPtr s,int i){var p=sqlite3_column_text(s,i); var b=new byte[sqlite3_column_bytes(s,i)];if(p==IntPtr.Zero)return "";Marshal.Copy(p,b,0,b.Length);return System.Text.Encoding.UTF8.GetString(b);}
}
'@
$dbHandle=[IntPtr]::Zero; $stmt=[IntPtr]::Zero
try {
 if([NativeSqlite]::sqlite3_open_v2([Text.Encoding]::UTF8.GetBytes($Db+[char]0),[ref]$dbHandle,1,[IntPtr]::Zero) -ne 0){throw 'open failed'}
 if([NativeSqlite]::sqlite3_prepare_v2($dbHandle,[Text.Encoding]::UTF8.GetBytes($Query+[char]0),-1,[ref]$stmt,[IntPtr]::Zero) -ne 0){throw 'query failed'}
 while([NativeSqlite]::sqlite3_step($stmt) -eq 100){$row=@();for($i=0;$i -lt [NativeSqlite]::sqlite3_column_count($stmt);$i++){$row += [NativeSqlite]::Read($stmt,$i)}; $row -join ' | '}
}finally{if($stmt -ne [IntPtr]::Zero){[void][NativeSqlite]::sqlite3_finalize($stmt)};if($dbHandle -ne [IntPtr]::Zero){[void][NativeSqlite]::sqlite3_close($dbHandle)}}
