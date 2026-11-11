package schedule

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/monbooru/monbooru/internal/config"
	"github.com/monbooru/monbooru/internal/galleryio"
	"github.com/monbooru/monbooru/internal/jobs"
	"github.com/monbooru/monbooru/internal/library"
	"github.com/monbooru/monbooru/internal/models"
	"github.com/monbooru/monbooru/internal/shared/fsx"
	"github.com/monbooru/monbooru/internal/shared/logx"
)

// Sortable as text, so the oldest export is the first name; seconds keep a
// manual run on the same day from overwriting the nightly one.
const exportStamp = "2006-01-02_150405"

// export writes one gallery's export into <export dir>/<gallery>/ and
// keeps only the newest schedule.export_keep files there.
func (s *Scheduler) export(r *jobs.Reservation, cx *library.Gallery) error {
	ctx, err := s.startPhase(r, models.JobTypeExport, "export", cx.Name)
	if err != nil {
		return err
	}
	cfg := s.host.Config()
	format := cfg.Schedule.ExportFormat
	dir := filepath.Join(cfg.ExportDir(), cx.Name)
	target := filepath.Join(dir, cx.Name+"-"+time.Now().Format(exportStamp)+exportSuffix(format))

	s.jobs.Update(0, 1, fmt.Sprintf("[%s] exporting…", cx.Name))
	err = s.writeExport(cx, format, target)
	if err == nil {
		pruneExports(dir, cx.Name, format, cfg.Schedule.ExportKeep)
	}
	if err := s.jobs.Settle(ctx, err,
		fmt.Sprintf("[%s] export cancelled", cx.Name),
		fmt.Sprintf("[%s] exported to %s", cx.Name, target)); err != nil {
		logx.Warnf("scheduler export %q: %v", cx.Name, err)
		return err
	}
	logx.Infof("scheduler: [%s] exported to %s", cx.Name, target)
	return nil
}

// writeExport pins the gallery against closing, as the web export does, and
// writes through a temp file, so a failed export never leaves a truncated
// file that looks whole.
func (s *Scheduler) writeExport(cx *library.Gallery, format, target string) error {
	s.set.RLock()
	cx.BeginExport()
	s.set.RUnlock()
	defer cx.EndExport()
	if cx.Closed() {
		return fmt.Errorf("gallery %q is closed", cx.Name)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return fsx.WriteAtomic(target, ".export.*", func(f *os.File) error {
		return exportTo(cx, format, f)
	})
}

func exportTo(cx *library.Gallery, format string, w io.Writer) error {
	switch format {
	case config.ExportFormatJSON:
		return galleryio.ExportGalleryJSON(cx.Handle, w, nil)
	case config.ExportFormatLight:
		return galleryio.ExportGalleryLightManifest(cx.Handle, w)
	default:
		return galleryio.ExportGalleryDB(cx.Handle, w)
	}
}

// Matches the web export's file names, so either kind imports the same way.
func exportSuffix(format string) string {
	switch format {
	case config.ExportFormatJSON:
		return ".json"
	case config.ExportFormatLight:
		return "-light.json"
	default:
		return ".db"
	}
}

// pruneExports deletes all but the newest keep exports of this format. Only
// files this phase names are touched, so whatever else the operator keeps
// in the folder stays.
func pruneExports(dir, gallery, format string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logx.Warnf("scheduler export %q: %v", gallery, err)
		return
	}
	prefix, suffix := gallery+"-", exportSuffix(format)
	var ours []string
	for _, e := range entries {
		name := e.Name()
		if e.Type().IsRegular() && isExportName(name, prefix, suffix) {
			ours = append(ours, name)
		}
	}
	slices.Sort(ours)
	for len(ours) > keep {
		if err := os.Remove(filepath.Join(dir, ours[0])); err != nil {
			logx.Warnf("scheduler export %q: %v", gallery, err)
		}
		ours = ours[1:]
	}
}

// The stamp must parse: "<gallery>-light.json" style names of other
// galleries, or a hand-named file, are not this phase's to delete.
func isExportName(name, prefix, suffix string) bool {
	stamp, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return false
	}
	stamp, ok = strings.CutSuffix(stamp, suffix)
	if !ok {
		return false
	}
	_, err := time.Parse(exportStamp, stamp)
	return err == nil
}
