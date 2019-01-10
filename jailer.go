// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package firecracker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/hashicorp/go-multierror"
)

const (
	defaultJailerPath = "/srv/jailer"
	defaultJailerBin  = "jailer"

	rootfsFolderName = "root"
)

// SeccompLevelValue represents a secure computing level type.
type SeccompLevelValue int

// secure computing levels
const (
	// SeccompLevelDisable is the default value.
	SeccompLevelDisable SeccompLevelValue = iota
	// SeccompLevelBasic prohibits syscalls not whitelisted by Firecracker.
	SeccompLevelBasic
	// SeccompLevelAdvanced adds further checks on some of the parameters of the
	// allowed syscalls.
	SeccompLevelAdvanced
)

// JailerConfig is jailer specific configuration needed to execute the jailer.
type JailerConfig struct {
	// GID the jailer switches to as it execs the target binary.
	GID *int

	// UID the jailer switches to as it execs the target binary.
	UID *int

	// ID is the unique VM identification string, which may contain alphanumeric
	// characters and hyphens. The maximum id length is currently 64 characters
	ID string

	// NumaNode represents the NUMA node the process gets assigned to.
	NumaNode *int

	// ExecFile is the path to the Firecracker binary that will be exec-ed by
	// the jailer. The user can provide a path to any binary, but the interaction
	// with the jailer is mostly Firecracker specific.
	ExecFile string

	// ChrootBaseDir represents the base folder where chroot jails are built. The
	// default is /srv/jailer
	ChrootBaseDir string

	// NetNS represents the path to a network namespace handle. If present, the
	// jailer will use this to join the associated network namespace
	NetNS string

	//  Daemonize is set to true, call setsid() and redirect STDIN, STDOUT, and
	//  STDERR to /dev/null
	Daemonize bool

	// SeccompLevel specifies whether seccomp filters should be installed and how
	// restrictive they should be. Possible values are:
	//
	//	0 : (default): disabled.
	//	1 : basic filtering. This prohibits syscalls not whitelisted by Firecracker.
	//	2 : advanced filtering. This adds further checks on some of the
	//			parameters of the allowed syscalls.
	SeccompLevel SeccompLevelValue

	// DevMapperStrategy will dictate how files are transfered to the root drive.
	DevMapperStrategy HandlersAdaptor
}

func (cfg JailerConfig) chrootBaseDir() string {
	if len(cfg.ChrootBaseDir) == 0 {
		return defaultJailerPath
	}

	return cfg.ChrootBaseDir
}

func (cfg JailerConfig) rootDir() string {
	return filepath.Join(cfg.chrootBaseDir(), "firecracker", cfg.ID, rootfsFolderName)
}

// JailerCommandBuilder will build a jailer command. This can be used to
// specify that a jailed firecracker executable wants to be run on the Machine.
type JailerCommandBuilder struct {
	bin      string
	id       string
	uid      int
	gid      int
	execFile string
	node     int

	// optional params
	chrootBaseDir string
	netNS         string
	daemonize     bool
	seccompLevel  SeccompLevelValue

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// Args returns the specified set of args to be used
// in command construction.
func (b JailerCommandBuilder) Args() []string {
	args := []string{}
	args = append(args, b.ID()...)
	args = append(args, b.UID()...)
	args = append(args, b.GID()...)
	args = append(args, b.ExecFile()...)
	args = append(args, b.NumaNode()...)

	if len(b.chrootBaseDir) > 0 {
		args = append(args, b.ChrootBaseDir()...)
	}

	if len(b.netNS) > 0 {
		args = append(args, b.NetNS()...)
	}

	args = append(args, b.SeccompLevel()...)

	if b.daemonize {
		args = append(args, "--daemonize")
	}

	return args
}

// Bin .
func (b JailerCommandBuilder) Bin() string {
	if len(b.bin) == 0 {
		return defaultJailerBin
	}

	return b.bin
}

// WithBin .
func (b JailerCommandBuilder) WithBin(bin string) JailerCommandBuilder {
	b.bin = bin
	return b
}

// ID will return the command arguments regarding the id.
func (b JailerCommandBuilder) ID() []string {
	return []string{
		"--id",
		b.id,
	}
}

// WithID will set the specified id to the builder.
func (b JailerCommandBuilder) WithID(id string) JailerCommandBuilder {
	b.id = id
	return b
}

// UID will return the command arguments regarding the uid.
func (b JailerCommandBuilder) UID() []string {
	return []string{
		"--uid",
		strconv.Itoa(b.uid),
	}
}

// WithUID will set the specified uid to the builder.
func (b JailerCommandBuilder) WithUID(uid int) JailerCommandBuilder {
	b.uid = uid
	return b
}

// GID will return the command arguments regarding the gid.
func (b JailerCommandBuilder) GID() []string {
	return []string{
		"--gid",
		strconv.Itoa(b.gid),
	}
}

// WithGID will set the specified gid to the builder.
func (b JailerCommandBuilder) WithGID(gid int) JailerCommandBuilder {
	b.gid = gid
	return b
}

// ExecFile will return the command arguments regarding the exec file.
func (b JailerCommandBuilder) ExecFile() []string {
	return []string{
		"--exec-file",
		b.execFile,
	}
}

// WithExecFile will set the specified path to the builder. This represents a
// firecracker binary used when calling the jailer.
func (b JailerCommandBuilder) WithExecFile(path string) JailerCommandBuilder {
	b.execFile = path
	return b
}

// NumaNode will return the command arguments regarding the numa node.
func (b JailerCommandBuilder) NumaNode() []string {
	return []string{
		"--node",
		strconv.Itoa(b.node),
	}
}

// WithNumaNode uses the specfied node for the jailer. This represents the numa
// node that the process will get assigned to.
func (b JailerCommandBuilder) WithNumaNode(node int) JailerCommandBuilder {
	b.node = node
	return b
}

// ChrootBaseDir will return the command arguments regarding the chroot base
// directory.
func (b JailerCommandBuilder) ChrootBaseDir() []string {
	return []string{
		"--chroot-base-dir",
		b.chrootBaseDir,
	}
}

// WithChrootBaseDir will set the given path as the chroot base directory. This
// specifies where chroot jails are built and defaults to /srv/jailer.
func (b JailerCommandBuilder) WithChrootBaseDir(path string) JailerCommandBuilder {
	b.chrootBaseDir = path
	return b
}

// NetNS will return the command arguments regarding the net namespace.
func (b JailerCommandBuilder) NetNS() []string {
	return []string{
		"--netns",
		b.netNS,
	}
}

// WithNetNS will set the given path to the net namespace of the builder. This
// represents the path to a network namespace handle and will be used to join
// the associated network namepsace.
func (b JailerCommandBuilder) WithNetNS(path string) JailerCommandBuilder {
	b.netNS = path
	return b
}

// WithDaemonize will specify whether to set stdio to /dev/null
func (b JailerCommandBuilder) WithDaemonize(daemonize bool) JailerCommandBuilder {
	b.daemonize = daemonize
	return b
}

// SeccompLevel will return the command arguments regarding secure computing
// level.
func (b JailerCommandBuilder) SeccompLevel() []string {
	return []string{
		"--seccomp-level",
		strconv.Itoa(int(b.seccompLevel)),
	}
}

// WithSeccompLevel will set the provided level to the builder. This represents
// the seccomp filters that should be installed and how restrictive they should
// be.
func (b JailerCommandBuilder) WithSeccompLevel(level SeccompLevelValue) JailerCommandBuilder {
	b.seccompLevel = level
	return b
}

// Stdout will return the stdout that will be used when creating the
// firecracker exec.Command
func (b JailerCommandBuilder) Stdout() io.Writer {
	return b.stdout
}

// WithStdout specifies which io.Writer to use in place of the os.Stdout in the
// firecracker exec.Command.
func (b JailerCommandBuilder) WithStdout(stdout io.Writer) JailerCommandBuilder {
	b.stdout = stdout
	return b
}

// Stderr will return the stderr that will be used when creating the
// firecracker exec.Command
func (b JailerCommandBuilder) Stderr() io.Writer {
	return b.stderr
}

// WithStderr specifies which io.Writer to use in place of the os.Stderr in the
// firecracker exec.Command.
func (b JailerCommandBuilder) WithStderr(stderr io.Writer) JailerCommandBuilder {
	b.stderr = stderr
	return b
}

// Stdin will return the stdin that will be used when creating the firecracker
// exec.Command
func (b JailerCommandBuilder) Stdin() io.Reader {
	return b.stdin
}

// WithStdin specifies which io.Reader to use in place of the os.Stdin in the
// firecracker exec.Command.
func (b JailerCommandBuilder) WithStdin(stdin io.Reader) JailerCommandBuilder {
	b.stdin = stdin
	return b
}

// Build will build a jailer command.
func (b JailerCommandBuilder) Build(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(
		ctx,
		b.Bin(),
		b.Args()...,
	)

	if stdin := b.Stdin(); stdin != nil {
		cmd.Stdin = stdin
	}

	if stdout := b.Stdout(); stdout != nil {
		cmd.Stdout = stdout
	}

	if stderr := b.Stderr(); stderr != nil {
		cmd.Stderr = stderr
	}

	fmt.Println("JAILER", cmd.Args)
	return cmd
}

// Jail will set up proper handlers and remove configuration validation due to
// stating of files
func jail(ctx context.Context, m *Machine, cfg *Config) error {
	chroot := cfg.JailerCfg.chrootBaseDir()
	rootfs := filepath.Join(chroot, "firecracker", cfg.JailerCfg.ID)

	cfg.SocketPath = filepath.Join(rootfs, "api.socket")
	m.cmd = JailerCommandBuilder{}.
		WithID(cfg.JailerCfg.ID).
		WithUID(*cfg.JailerCfg.UID).
		WithGID(*cfg.JailerCfg.GID).
		WithNumaNode(*cfg.JailerCfg.NumaNode).
		WithExecFile(cfg.JailerCfg.ExecFile).
		WithChrootBaseDir(cfg.JailerCfg.ChrootBaseDir).
		WithDaemonize(cfg.JailerCfg.Daemonize).
		WithSeccompLevel(cfg.JailerCfg.SeccompLevel).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		Build(ctx)

	if err := cfg.JailerCfg.DevMapperStrategy.AdaptHandlers(&m.Handlers); err != nil {
		return err
	}

	return nil
}

func linkFileToRootFS(cfg JailerConfig, dst, src string) error {
	if err := os.Link(src, dst); err != nil {
		return err
	}

	return os.Chown(dst, *cfg.UID, *cfg.GID)
}

// LinkFilesHandler creates a new link files handler that will link files to
// the rootfs
func LinkFilesHandler(rootfs, kernelImageFileName string) Handler {
	return Handler{
		Name: LinkFilesToRootFSHandlerName,
		Fn: func(ctx context.Context, m *Machine) error {
			// copy kernel image to root fs
			if err := linkFileToRootFS(
				m.cfg.JailerCfg,
				filepath.Join(rootfs, kernelImageFileName),
				m.cfg.KernelImagePath,
			); err != nil {
				return err
			}

			// copy all drives to the root fs
			for i, drive := range m.cfg.Drives {
				hostPath := StringValue(drive.PathOnHost)
				driveFileName := filepath.Base(hostPath)

				if err := linkFileToRootFS(
					m.cfg.JailerCfg,
					filepath.Join(rootfs, driveFileName),
					hostPath,
				); err != nil {
					return err
				}

				m.cfg.Drives[i].PathOnHost = String(driveFileName)
			}

			m.cfg.KernelImagePath = kernelImageFileName
			return nil
		},
	}
}

// NaiveDevMapperStrategy will simply hard link all files, drives and kernel
// image, to the root drive.
type NaiveDevMapperStrategy struct {
	Rootfs          string
	KernelImagePath string
}

// NewNaiveDevMapperStrategy returns a new NaivceDevMapperStrategy
func NewNaiveDevMapperStrategy(rootfs, kernelImagePath string) NaiveDevMapperStrategy {
	return NaiveDevMapperStrategy{
		Rootfs:          rootfs,
		KernelImagePath: kernelImagePath,
	}
}

// ErrCreateMachineHandlerMissing occurs when the CreateMachineHandler is not
// present in FcInit.
var ErrCreateMachineHandlerMissing = fmt.Errorf("%s is missing from FcInit's list", CreateMachineHandlerName)

// AdaptHandlers will inject the LinkFilesHandler into the handler list.
func (s NaiveDevMapperStrategy) AdaptHandlers(handlers *Handlers) error {
	if !handlers.FcInit.Has(CreateMachineHandlerName) {
		return ErrCreateMachineHandlerMissing
	}

	handlers.FcInit = handlers.FcInit.AppendAfter(
		CreateMachineHandlerName,
		LinkFilesHandler(filepath.Join(s.Rootfs, rootfsFolderName), filepath.Base(s.KernelImagePath)),
	)

	return nil
}

// BindMountDevMapperStrategy will use the syscall.Mount function to bind a
// mount to the root drive.
type BindMountDevMapperStrategy struct {
	data string
}

// NewBindMountDevMapperStrategy returns a new BindMountDevMapperStrategy that
// can be used to bind mounts to the firecracker VMM.
func NewBindMountDevMapperStrategy() BindMountDevMapperStrategy {
	return BindMountDevMapperStrategy{
		data: "gid=100,uid=123",
	}
}

// AdaptHandlers will inject the appropriate handler used to bind a mount. This
// handlers will inject after the CreateMachineHandler, and if that handler
// does not exist, an error will be returned.
func (s BindMountDevMapperStrategy) AdaptHandlers(handlers *Handlers) error {
	if !handlers.FcInit.Has(CreateMachineHandlerName) {
		return ErrCreateMachineHandlerMissing
	}

	handlers.FcInit = handlers.FcInit.AppendAfter(
		CreateMachineHandlerName,
		Handler{
			Name: "MountToRootFS",
			Fn:   s.handler,
		},
	)

	handlers.Finish = handlers.Finish.Swappend(Handler{
		Name: "finish.umountDrive",
		Fn: func(ctx context.Context, m *Machine) error {
			rootDir := m.cfg.JailerCfg.rootDir()
			kernelImagePath := filepath.Join(rootDir, m.cfg.KernelImagePath)

			var errs *multierror.Error
			if err := syscall.Unmount(kernelImagePath, syscall.MNT_FORCE); err != nil {
				multierror.Append(errs, err)
			}

			for _, drive := range m.cfg.Drives {
				if err := syscall.Unmount(
					filepath.Join(rootDir, StringValue(drive.PathOnHost)),
					syscall.MNT_FORCE,
				); err != nil {
					multierror.Append(errs, err)
				}
			}

			return errs.ErrorOrNil()
		},
	})

	return nil
}

// TODO: add tests
func (s BindMountDevMapperStrategy) handler(ctx context.Context, m *Machine) error {
	rootDir := m.cfg.JailerCfg.rootDir()
	kernelImageName := filepath.Base(m.cfg.KernelImagePath)
	kernelImagePath := filepath.Join(rootDir, kernelImageName)
	uid := *m.cfg.JailerCfg.UID
	gid := *m.cfg.JailerCfg.GID

	/*mtr := newMounter(uid, gid, mntNSType)
	defer mtr.Close()

	if err := mtr.EnterNS(m.cmd.Process.Pid); err != nil {
		return err
	}

	if err := mtr.Mount(m.cfg.KernelImagePath, kernelImagePath, true); err != nil {
		return err
	}

	m.cfg.KernelImagePath = kernelImageName

	for i, drive := range m.cfg.Drives {
		hostPath := StringValue(drive.PathOnHost)
		driveFileName := filepath.Base(hostPath)
		mountDriveFilePath := filepath.Join(rootDir, driveFileName)

		if err := mtr.Mount(StringValue(drive.PathOnHost), mountDriveFilePath, BoolValue(drive.IsReadOnly)); err != nil {
			return err
		}

		m.cfg.Drives[i].PathOnHost = String(driveFileName)
	}*/

	if err := bindMount(m.cfg.KernelImagePath, kernelImagePath, m.cmd.Process.Pid, uid, gid); err != nil {
		return fmt.Errorf("failed to mount kernel image: %v", err)
	}

	m.cfg.KernelImagePath = kernelImageName
	fmt.Println("KERNEL PATH", m.cfg.KernelImagePath)

	for i, drive := range m.cfg.Drives {
		hostPath := StringValue(drive.PathOnHost)
		driveFileName := filepath.Base(hostPath)
		mountDriveFilePath := filepath.Join(rootDir, driveFileName)

		if err := bindMount(
			StringValue(drive.PathOnHost),
			mountDriveFilePath,
			m.cmd.Process.Pid,
			uid,
			gid,
		); err != nil {
			return fmt.Errorf("failed to mount drive %q: %v", mountDriveFilePath, err)
		}

		m.cfg.Drives[i].PathOnHost = String(driveFileName)

		fmt.Println("DRIVE PATH", *m.cfg.Drives[i].PathOnHost)
	}

	return nil
}

const nsenterBin = "nsenter"

func bindMount(src, target string, pid, uid, gid int) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("%s could not be found", src)
	}

	fmt.Println("SRC TARGET", src, target)
	cmd := exec.Command(
		nsenterBin,
		"-t",
		strconv.Itoa(pid),
		fmt.Sprintf("sudo touch %s", target),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("ARGSSSSSSSSSSSSSSSSS", cmd.Args)
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command(
		nsenterBin,
		"-m",
		"-t",
		strconv.Itoa(pid),
		fmt.Sprintf("sudo mount --bind %s %s", src, target),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("ARGSSSSSSSSSSSSSSSSS", cmd.Args)
	return cmd.Run()
}
