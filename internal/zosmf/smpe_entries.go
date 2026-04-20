package zosmf

import "fmt"

// ValidEntryTypes maps each SMP/E CSI entry type to its valid subentry names.
// Source: IBM z/OS 3.1 SMP/E Reference.
var ValidEntryTypes = map[string][]string{
	"SYSMOD":     {"ACCEPT", "ACCID", "APPID", "APPLY", "ASSEM", "BYPASS", "CIFREQ", "DELBY", "DELETE", "DELLMOD", "DESCRIPTION", "DLMOD", "ELEMENT", "ELEMMOV", "EMOVE", "ENAME", "ERROR", "FEATURE", "FESN", "FMID", "HOLDDATA", "IFREQ", "INSTALLDATE", "INSTALLTIME", "JAR", "JARUPD", "JCLIN", "LASTSUP", "LASTUPD", "LASTUPDTYPE", "MAC", "MACUPD", "MOD", "NPRE", "PRE", "PROGRAM", "RECDATE", "RECTIME", "REGEN", "RENLMOD", "REQ", "RESDATE", "RESTIME", "RESTORE", "REWORK", "RLMOD", "SMODTYPE", "SOURCEID", "SRC", "SRCUPD", "SREL", "SUPBY", "SUPING", "SZAP", "TLIBPREFIX", "UCLDATE", "UCLTIME", "VERSION", "XZAP"},
	"DDDEF":      {"CONCAT", "DATACLAS", "DATASET", "DIR", "DISP", "DSNTYPE", "DSPREFIX", "ENAME", "INITDISP", "MGMTCLAS", "PATH", "PROTECT", "SPACE", "STORCLAS", "SYSOUT", "UNIT", "UNITS", "VOLUME", "WAITFORDSN"},
	"TARGETZONE": {"ENAME", "OPTIONS", "RELATED", "SREL", "TIEDTO", "UPGLEVEL", "XZLINK", "ZDESC"},
	"DLIB":       {"ENAME", "LASTUPD", "LASTUPDTYPE", "SYSLIB"},
	"GLOBALZONE": {"ENAME", "FMID", "OPTIONS", "SREL", "UPGLEVEL", "ZDESC", "ZONEINDEX"},
	"ASSEM":      {"ASMIN", "ENAME", "LASTUPD", "LASTUPDTYPE"},
	"DATA":       {"ALIAS", "DISTLIB", "ENAME", "FMID", "LASTUPD", "LASTUPDTYPE", "RMID", "SYSLIB"},
	"LMOD":       {"CALLLIBS", "COPIED", "ENAME", "LASTUPD", "LASTUPDTYPE", "LEPARM", "LECNTL", "LMODALIAS", "LMODSYMLINK", "MODDEL", "RC", "SIDEDECKLIB", "SYSLIB", "UTIN", "XZMOD", "XZMODP"},
	"MAC":        {"DISTLIB", "ENAME", "FMID", "GENASM", "LASTUPD", "LASTUPDTYPE", "MALIAS", "RMID", "SYSLIB", "UMID"},
	"MOD":        {"ASSEMBLE", "CSECT", "DALIAS", "DISTLIB", "ENAME", "FMID", "LASTUPD", "LASTUPDTYPE", "LEPARM", "LMOD", "RMID", "RMIDASM", "TALIAS", "UMID", "XZLMOD", "XZLMODP"},
	"SRC":        {"DISTLIB", "ENAME", "FMID", "LASTUPD", "LASTUPDTYPE", "RMID", "SYSLIB", "UMID"},
	"JAR":        {"DISTLIB", "ENAME", "FMID", "JARPARM", "LASTUPD", "LASTUPDTYPE", "LINK", "RMID", "SHSCRIPT", "SYMLINK", "SYMPATH", "SYSLIB", "TXLIB", "UMID", "VERSION"},
	"JARUPD":     {"ENAME", "JARPARM", "LASTUPD", "LASTUPDTYPE", "LINK", "SYMLINK", "SYMPATH", "TXLIB"},
	"PROGRAM":    {"ALIAS", "DISTLIB", "ENAME", "FMID", "LASTUPD", "LASTUPDTYPE", "LKLIB", "RMID", "SYSLIB", "VERSION"},
	"DLIBZONE":   {"ACCJCLIN", "ENAME", "OPTIONS", "RELATED", "SREL", "UPGLEVEL", "ZDESC"},
	"FEATURE":    {"DESCRIPTION", "ENAME", "FMID", "PRODUCT", "RECDATE", "RECTIME", "REWORK", "UCLDATE", "UCLTIME"},
	"FMIDSET":    {"ENAME", "FMID"},
	"HOLDDATA":   {"ENAME", "HOLDCLASS", "HOLDDATA", "HOLDDATE", "HOLDFIXCAT", "HOLDFMID", "HOLDREASON", "HOLDRESOLVER", "HOLDTYPE"},
	"OPTIONS":    {"AMS", "ASM", "CHANGEFILE", "COMP", "COMPACT", "COPY", "DSPREFIX", "DSSPACE", "ENAME", "EXRTYDD", "FIXCAT", "HFSCOPY", "IOSUP", "LKED", "MSGFILTER", "MSGWIDTH", "NOPURGE", "NOREJECT", "ORDERRET", "PAGELEN", "PEMAX", "RECZGRP", "RECEXCGRP", "RETRY", "RETRYDDN", "SAVEMTS", "SAVESTS", "SUPPHOLD", "UPDATE", "ZAP"},
	"ORDER":      {"APARS", "CONTENT", "DOWNLDATE", "DOWNLTIME", "ENAME", "ORDERDATE", "ORDERID", "ORDERSERVER", "ORDERTIME", "PKGID", "PTFS", "STATUS", "USERID", "ZONES"},
	"PRODUCT":    {"DESCRIPTION", "PRODID", "PRODSUP", "RECDATE", "RECTIME", "REWORK", "SREL", "UCLDATE", "UCLTIME", "URL", "VENDOR", "VRM"},
	"UTILITY":    {"ENAME", "LIST", "NAME", "PRINT", "RC", "UTILPARM"},
	"ZONESET":    {"ENAME", "XZREQCHK", "ZONENAME"},
}

// DefaultSubentries defines the subentries returned by default when no explicit subentries are requested.
// These are chosen to provide the most useful information for each entry type.
var DefaultSubentries = map[string][]string{
	"SYSMOD":     {"APPID", "APPLY", "ERROR", "HOLDDATA", "IFREQ", "INSTALLDATE", "INSTALLTIME", "LASTUPD", "PRE", "RECDATE", "RECTIME", "REQ", "REWORK", "SUPBY", "SUPING"},
	"DDDEF":      {"CONCAT", "DATACLAS", "DATASET", "DISP", "DSNTYPE", "INITDISP", "MGMTCLAS", "PATH", "STORCLAS"},
	"TARGETZONE": {"ENAME", "OPTIONS", "RELATED", "SREL", "TIEDTO", "UPGLEVEL", "XZLINK", "ZDESC"},
	"DLIB":       {"ENAME", "LASTUPD", "LASTUPDTYPE", "SYSLIB"},
	"GLOBALZONE": {"ENAME", "FMID", "OPTIONS", "UPGLEVEL", "ZDESC", "ZONEINDEX"},
	"DLIBZONE":   {"ACCJCLIN", "ENAME", "OPTIONS", "RELATED", "SREL", "UPGLEVEL", "ZDESC"},
	"ASSEM":      {"ASMIN", "LASTUPD"},
	"DATA":       {"DISTLIB", "FMID", "SYSLIB"},
	"LMOD":       {"SYSLIB", "LEPARM", "LMODALIAS"},
	"MAC":        {"DISTLIB", "FMID", "SYSLIB", "MALIAS"},
	"MOD":        {"FMID", "LMOD", "CSECT", "DISTLIB"},
	"SRC":        {"DISTLIB", "FMID", "SYSLIB"},
	"JAR":        {"DISTLIB", "FMID", "SYSLIB", "VERSION"},
	"JARUPD":     {"JARPARM", "LASTUPD"},
	"PROGRAM":    {"DISTLIB", "FMID", "SYSLIB", "VERSION"},
	"FEATURE":    {"FMID", "PRODUCT", "DESCRIPTION"},
	"FMIDSET":    {"FMID"},
	"HOLDDATA":   {"HOLDCLASS", "HOLDREASON", "HOLDDATE", "HOLDFIXCAT", "HOLDRESOLVER"},
	"OPTIONS":    {"DSPREFIX", "DSSPACE", "COMPACT", "NOPURGE", "NOREJECT"},
	"ORDER":      {"ORDERID", "ORDERDATE", "STATUS", "PTFS", "APARS"},
	"PRODUCT":    {"PRODID", "VRM", "DESCRIPTION", "VENDOR", "SREL"},
	"UTILITY":    {"NAME", "RC", "UTILPARM"},
	"ZONESET":    {"ZONENAME"},
}

// hfsSubentries are shared by all HFS element types (AIX1-5, CLIENT1-5, OS21-5, UNIX1-5, WIN1-5).
var hfsSubentries = []string{"DISTLIB", "ENAME", "FMID", "HFSPARM", "INSTMODE", "LASTUPD", "LASTUPDTYPE", "LINK", "RMID", "SHSCRIPT", "SYMLINK", "SYMPATH", "SYSLIB"}
var hfsDefaultSubentries = []string{"DISTLIB", "FMID", "SYSLIB", "SYMLINK", "SYMPATH"}

func init() {
	// Register all HFS element types with shared subentries and defaults.
	for _, prefix := range []string{"AIX", "CLIENT", "OS2", "UNIX", "WIN"} {
		for i := 1; i <= 5; i++ {
			name := fmt.Sprintf("%s%d", prefix, i)
			ValidEntryTypes[name] = hfsSubentries
			DefaultSubentries[name] = hfsDefaultSubentries
		}
	}
}
