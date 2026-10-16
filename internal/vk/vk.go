//go:build (windows || linux || darwin) && (amd64 || arm64)

// Package vk is a minimal, cgo-free Vulkan compute binding: just enough to
// create a device, upload storage buffers, and dispatch one compute shader.
// The Vulkan loader is opened at runtime, so binaries built with it still run
// on machines without Vulkan as long as this package is not used.
package vk

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"time"
	"unsafe"
)

func check(r Result, what string) error {
	if r >= 0 {
		return nil
	}
	if r == resultErrorDeviceLost {
		return fmt.Errorf("%s: VK_ERROR_DEVICE_LOST (driver reset or GPU hang)", what)
	}
	return fmt.Errorf("%s failed: VkResult %d", what, r)
}

func cstr(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}

// cstrs builds a C array of C strings. The returned slice must stay alive
// (and be pinned) for the duration of the call that uses it.
func cstrs(p *runtime.Pinner, ss []string) **byte {
	if len(ss) == 0 {
		return nil
	}
	arr := make([]*byte, len(ss))
	for i, s := range ss {
		arr[i] = cstr(s)
		p.Pin(arr[i])
	}
	p.Pin(&arr[0])
	return &arr[0]
}

// DeviceType mirrors VkPhysicalDeviceType.
type DeviceType uint32

func (t DeviceType) String() string {
	switch t {
	case 1:
		return "integrated GPU"
	case 2:
		return "discrete GPU"
	case 3:
		return "virtual GPU"
	case 4:
		return "CPU"
	default:
		return "other"
	}
}

// rank orders device types for automatic selection.
func (t DeviceType) rank() int {
	switch t {
	case 2:
		return 4
	case 1:
		return 3
	case 3:
		return 2
	case 4:
		return 1
	default:
		return 0
	}
}

// DeviceInfo describes a physical device.
type DeviceInfo struct {
	Index       int
	Name        string
	Type        DeviceType
	VendorID    uint32
	APIVersion  uint32
	ShaderInt64 bool
	Compute     bool

	// Optional capabilities (zero on Vulkan 1.0 devices or loaders).
	SubgroupSize        uint32 // default subgroup size reported by the driver
	SubgroupSizeControl bool   // compute pipelines can require a subgroup size
	MinSubgroupSize     uint32 // valid when SubgroupSizeControl is set
	MaxSubgroupSize     uint32
	PipelineStatistics  bool // VK_KHR_pipeline_executable_properties

	handle         uintptr
	family         uint32
	extensions     map[string]bool
	sscExtension   bool   // enable VK_EXT_subgroup_size_control (device below Vulkan 1.3)
	maxWGSubgroups uint32 // maxComputeWorkgroupSubgroups
}

// Usable reports whether the device can run the compute shaders this
// program uses (they need 64-bit integer arithmetic).
func (d DeviceInfo) Usable() bool { return d.Compute && d.ShaderInt64 }

// Instance is a Vulkan instance.
type Instance struct {
	handle  uintptr
	version uint32 // VkApplicationInfo::apiVersion the instance was created with
	devices []DeviceInfo
}

// NewInstance loads the Vulkan loader and creates an instance.
func NewInstance(appName string) (*Instance, error) {
	if err := loadGlobal(); err != nil {
		return nil, err
	}

	var pin runtime.Pinner
	defer pin.Unpin()

	var exts []string
	var flags uint32
	if instanceExtensions()[extPortabilityEnumeration] {
		// Needed to see MoltenVK (and other non-conformant) devices.
		exts = append(exts, extPortabilityEnumeration)
		flags |= instanceCreateEnumeratePortabilityBit
	}

	// A 1.0 loader rejects any other apiVersion; 1.1+ loaders accept any
	// value, and the effective device version is min(this, device version).
	version := uint32(apiVersion10)
	if vkEnumerateInstanceVersion != nil {
		var v uint32
		if vkEnumerateInstanceVersion(&v) == 0 && v >= apiVersion11 {
			version = apiVersion13
		}
	}
	app := &applicationInfo{
		sType:            stApplicationInfo,
		pApplicationName: cstr(appName),
		apiVersion:       version,
	}
	pin.Pin(app.pApplicationName)
	pin.Pin(app)
	info := &instanceCreateInfo{
		sType:                   stInstanceCreateInfo,
		flags:                   flags,
		pApplicationInfo:        app,
		enabledExtensionCount:   uint32(len(exts)),
		ppEnabledExtensionNames: cstrs(&pin, exts),
	}
	var handle uintptr
	if err := check(vkCreateInstance(info, nil, &handle), "vkCreateInstance"); err != nil {
		return nil, err
	}
	if err := loadInstance(handle); err != nil {
		return nil, err
	}
	in := &Instance{handle: handle, version: version}
	devs, err := in.enumerate()
	if err != nil {
		in.Destroy()
		return nil, err
	}
	in.devices = devs
	return in, nil
}

func instanceExtensions() map[string]bool {
	var n uint32
	if vkEnumerateInstanceExtensionProperties(nil, &n, nil) < 0 || n == 0 {
		return nil
	}
	props := make([]extensionProperties, n)
	if vkEnumerateInstanceExtensionProperties(nil, &n, &props[0]) < 0 {
		return nil
	}
	return extensionSet(props[:n])
}

func extensionSet(props []extensionProperties) map[string]bool {
	set := make(map[string]bool, len(props))
	for _, p := range props {
		set[cstring(p.extensionName[:])] = true
	}
	return set
}

func cstring(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func (in *Instance) enumerate() ([]DeviceInfo, error) {
	var n uint32
	if err := check(vkEnumeratePhysicalDevices(in.handle, &n, nil), "vkEnumeratePhysicalDevices"); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	handles := make([]uintptr, n)
	if err := check(vkEnumeratePhysicalDevices(in.handle, &n, &handles[0]), "vkEnumeratePhysicalDevices"); err != nil {
		return nil, err
	}
	devs := make([]DeviceInfo, 0, n)
	for i, h := range handles[:n] {
		var props physicalDeviceProperties
		vkGetPhysicalDeviceProperties(h, &props)
		var feats physicalDeviceFeatures
		vkGetPhysicalDeviceFeatures(h, &feats)
		family, ok := computeFamily(h)

		var extCount uint32
		var exts map[string]bool
		if vkEnumerateDeviceExtensionProperties(h, nil, &extCount, nil) >= 0 && extCount > 0 {
			props := make([]extensionProperties, extCount)
			if vkEnumerateDeviceExtensionProperties(h, nil, &extCount, &props[0]) >= 0 {
				exts = extensionSet(props[:extCount])
			}
		}

		info := DeviceInfo{
			Index:       i,
			Name:        cstring(props.deviceName[:]),
			Type:        DeviceType(props.deviceType),
			VendorID:    props.vendorID,
			APIVersion:  props.apiVersion,
			ShaderInt64: feats[featureShaderInt64] != 0,
			Compute:     ok,
			handle:      h,
			family:      family,
			extensions:  exts,
		}
		in.queryOptional(&info)
		devs = append(devs, info)
	}
	return devs, nil
}

// queryOptional fills the optional capabilities through the Vulkan 1.1
// query commands. Only structures of supported versions/extensions are
// chained.
func (in *Instance) queryOptional(info *DeviceInfo) {
	if in.version < apiVersion11 || info.APIVersion < apiVersion11 ||
		vkGetPhysicalDeviceFeatures2 == nil || vkGetPhysicalDeviceProperties2 == nil {
		return
	}
	var pin runtime.Pinner
	defer pin.Unpin()

	sscCore := min(in.version, info.APIVersion) >= apiVersion13
	sscExt := !sscCore && info.extensions[extSubgroupSizeControl]
	pep := info.extensions[extPipelineExecutableProperties]

	sgProps := &physicalDeviceSubgroupProperties{sType: stPhysicalDeviceSubgroupProperties}
	sscProps := &physicalDeviceSubgroupSizeControlProperties{sType: stPhysicalDeviceSubgroupSizeControlProperties}
	sscFeats := &physicalDeviceSubgroupSizeControlFeatures{sType: stPhysicalDeviceSubgroupSizeControlFeatures}
	pepFeats := &physicalDevicePipelineExecutablePropertiesFeatures{sType: stPhysicalDevicePipelineExecutablePropsFeatures}
	pin.Pin(sgProps)
	pin.Pin(sscProps)
	pin.Pin(sscFeats)
	pin.Pin(pepFeats)

	props2 := &physicalDeviceProperties2{sType: stPhysicalDeviceProperties2, pNext: unsafe.Pointer(sgProps)}
	feats2 := &physicalDeviceFeatures2{sType: stPhysicalDeviceFeatures2}
	if sscCore || sscExt {
		sgProps.pNext = unsafe.Pointer(sscProps)
		feats2.pNext = unsafe.Pointer(sscFeats)
	}
	if pep {
		pepFeats.pNext = feats2.pNext
		feats2.pNext = unsafe.Pointer(pepFeats)
	}
	vkGetPhysicalDeviceProperties2(info.handle, props2)
	vkGetPhysicalDeviceFeatures2(info.handle, feats2)

	info.SubgroupSize = sgProps.subgroupSize
	if (sscCore || sscExt) && sscFeats.subgroupSizeControl != 0 &&
		sscProps.requiredSubgroupSizeStages&shaderStageComputeBit != 0 {
		info.SubgroupSizeControl = true
		info.MinSubgroupSize = sscProps.minSubgroupSize
		info.MaxSubgroupSize = sscProps.maxSubgroupSize
		info.maxWGSubgroups = sscProps.maxComputeWorkgroupSubgroups
		info.sscExtension = sscExt
	}
	info.PipelineStatistics = pep && pepFeats.pipelineExecutableInfo != 0
}

// computeFamily picks a queue family with compute support, preferring a
// dedicated (non-graphics) one so long dispatches do not stall the desktop.
func computeFamily(pd uintptr) (uint32, bool) {
	var n uint32
	vkGetPhysicalDeviceQueueFamilyProperties(pd, &n, nil)
	if n == 0 {
		return 0, false
	}
	fams := make([]queueFamilyProperties, n)
	vkGetPhysicalDeviceQueueFamilyProperties(pd, &n, &fams[0])
	best, found := uint32(0), false
	for i, f := range fams[:n] {
		if f.queueFlags&queueComputeBit == 0 || f.queueCount == 0 {
			continue
		}
		if f.queueFlags&queueGraphicsBit == 0 {
			return uint32(i), true
		}
		if !found {
			best, found = uint32(i), true
		}
	}
	return best, found
}

// Devices lists the physical devices in loader order.
func (in *Instance) Devices() []DeviceInfo { return in.devices }

// BestDevice returns the index of the most capable usable device, preferring
// discrete GPUs over integrated ones over software rasterizers.
func (in *Instance) BestDevice() (int, error) {
	best := -1
	for i, d := range in.devices {
		if d.Usable() && (best < 0 || d.Type.rank() > in.devices[best].Type.rank()) {
			best = i
		}
	}
	if best < 0 {
		if len(in.devices) == 0 {
			return -1, errors.New("no Vulkan devices found")
		}
		return -1, errors.New("no Vulkan device supports compute with 64-bit integers (shaderInt64)")
	}
	return best, nil
}

// Destroy destroys the instance.
func (in *Instance) Destroy() {
	if in.handle != 0 {
		vkDestroyInstance(in.handle, nil)
		in.handle = 0
	}
}

// Device is a logical device with one compute queue, one command buffer
// and one fence. It is not safe for concurrent use.
type Device struct {
	Info     DeviceInfo
	Options  DeviceOptions // the optional features actually enabled
	handle   uintptr
	queue    uintptr
	memProps physicalDeviceMemoryProperties
	cmdPool  uint64
	cmd      uintptr
	fence    uint64
}

// DeviceOptions selects optional features. The zero value enables no optional
// features or extensions (apart from VK_KHR_portability_subset when the device
// advertises it). Note that the instance targets Vulkan 1.3 whenever the
// loader is 1.1 or newer, so devices run at min(1.3, their own version), and
// enumeration queries 1.1+ devices with vkGetPhysicalDeviceFeatures2/Properties2.
type DeviceOptions struct {
	SubgroupSizeControl bool // needed for KernelOptions.SubgroupSize
	PipelineStatistics  bool // needed for KernelOptions.CaptureStatistics
}

// Open creates a logical device on the physical device with the given index.
// Requested options the device does not support are an error.
func (in *Instance) Open(index int, opts DeviceOptions) (*Device, error) {
	if index < 0 || index >= len(in.devices) {
		return nil, fmt.Errorf("no Vulkan device with index %d (found %d)", index, len(in.devices))
	}
	info := in.devices[index]
	if !info.Compute {
		return nil, fmt.Errorf("%s has no compute queue", info.Name)
	}
	if !info.ShaderInt64 {
		return nil, fmt.Errorf("%s does not support 64-bit integers in shaders (shaderInt64)", info.Name)
	}

	var pin runtime.Pinner
	defer pin.Unpin()

	priority := new(float32)
	*priority = 1
	pin.Pin(priority)
	queueInfo := &deviceQueueCreateInfo{
		sType:            stDeviceQueueCreateInfo,
		queueFamilyIndex: info.family,
		queueCount:       1,
		pQueuePriorities: priority,
	}
	pin.Pin(queueInfo)
	features := new(physicalDeviceFeatures)
	features[featureShaderInt64] = 1
	pin.Pin(features)
	var exts []string
	if info.extensions[extPortabilitySubset] {
		exts = append(exts, extPortabilitySubset) // required when the device advertises it
	}
	// Extension feature structures go straight into VkDeviceCreateInfo's
	// pNext chain, which is valid next to pEnabledFeatures.
	var next unsafe.Pointer
	if opts.SubgroupSizeControl {
		if !info.SubgroupSizeControl {
			return nil, fmt.Errorf("%s cannot set the compute subgroup size", info.Name)
		}
		f := &physicalDeviceSubgroupSizeControlFeatures{sType: stPhysicalDeviceSubgroupSizeControlFeatures, pNext: next, subgroupSizeControl: 1}
		pin.Pin(f)
		next = unsafe.Pointer(f)
		if info.sscExtension {
			exts = append(exts, extSubgroupSizeControl)
		}
	}
	if opts.PipelineStatistics {
		if !info.PipelineStatistics {
			return nil, fmt.Errorf("%s does not report pipeline statistics", info.Name)
		}
		f := &physicalDevicePipelineExecutablePropertiesFeatures{sType: stPhysicalDevicePipelineExecutablePropsFeatures, pNext: next, pipelineExecutableInfo: 1}
		pin.Pin(f)
		next = unsafe.Pointer(f)
		exts = append(exts, extPipelineExecutableProperties)
	}
	createInfo := &deviceCreateInfo{
		sType:                   stDeviceCreateInfo,
		pNext:                   next,
		queueCreateInfoCount:    1,
		pQueueCreateInfos:       queueInfo,
		enabledExtensionCount:   uint32(len(exts)),
		ppEnabledExtensionNames: cstrs(&pin, exts),
		pEnabledFeatures:        features,
	}

	d := &Device{Info: info, Options: opts}
	if err := check(vkCreateDevice(info.handle, createInfo, nil, &d.handle), "vkCreateDevice"); err != nil {
		return nil, err
	}
	vkGetDeviceQueue(d.handle, info.family, 0, &d.queue)
	vkGetPhysicalDeviceMemoryProperties(info.handle, &d.memProps)

	poolInfo := &commandPoolCreateInfo{
		sType:            stCommandPoolCreateInfo,
		flags:            commandPoolCreateResetCommandBufferBit,
		queueFamilyIndex: info.family,
	}
	if err := check(vkCreateCommandPool(d.handle, poolInfo, nil, &d.cmdPool), "vkCreateCommandPool"); err != nil {
		d.Destroy()
		return nil, err
	}
	allocInfo := &commandBufferAllocateInfo{
		sType:              stCommandBufferAllocateInfo,
		commandPool:        d.cmdPool,
		commandBufferCount: 1,
	}
	if err := check(vkAllocateCommandBuffers(d.handle, allocInfo, &d.cmd), "vkAllocateCommandBuffers"); err != nil {
		d.Destroy()
		return nil, err
	}
	if err := check(vkCreateFence(d.handle, &fenceCreateInfo{sType: stFenceCreateInfo}, nil, &d.fence), "vkCreateFence"); err != nil {
		d.Destroy()
		return nil, err
	}
	return d, nil
}

// WaitIdle blocks until all work submitted to the device has finished,
// e.g. after Dispatch gave up waiting, before buffers or kernels are destroyed.
func (d *Device) WaitIdle() {
	if d.handle != 0 {
		vkDeviceWaitIdle(d.handle)
	}
}

// Destroy waits for the device to go idle and destroys it.
func (d *Device) Destroy() {
	if d.handle == 0 {
		return
	}
	vkDeviceWaitIdle(d.handle)
	if d.fence != 0 {
		vkDestroyFence(d.handle, d.fence, nil)
	}
	if d.cmdPool != 0 {
		vkDestroyCommandPool(d.handle, d.cmdPool, nil)
	}
	vkDestroyDevice(d.handle, nil)
	d.handle = 0
}

func (d *Device) memoryType(typeBits, required, preferred uint32) (uint32, bool) {
	for _, want := range []uint32{required | preferred, required} {
		for i := range d.memProps.memoryTypeCount {
			flags := d.memProps.memoryTypes[i].propertyFlags
			if typeBits&(1<<i) != 0 && flags&want == want {
				return i, true
			}
		}
	}
	return 0, false
}

// Buffer is a host-visible, host-coherent storage buffer that stays mapped
// for its whole lifetime. Data aliases the mapped memory.
type Buffer struct {
	Data   []byte
	dev    *Device
	handle uint64
	memory uint64
}

// NewBuffer creates a mapped storage buffer of the given size. If
// deviceLocal is set, memory that is both host-visible and in VRAM is
// preferred (good for read-mostly inputs); otherwise host-cached memory is
// preferred (good for results the host reads back).
func (d *Device) NewBuffer(size int, deviceLocal bool) (*Buffer, error) {
	info := &bufferCreateInfo{
		sType: stBufferCreateInfo,
		size:  uint64(size),
		usage: bufferUsageStorageBufferBit,
	}
	b := &Buffer{dev: d}
	if err := check(vkCreateBuffer(d.handle, info, nil, &b.handle), "vkCreateBuffer"); err != nil {
		return nil, err
	}
	var req memoryRequirements
	vkGetBufferMemoryRequirements(d.handle, b.handle, &req)
	preferred := uint32(memoryPropertyHostCachedBit)
	if deviceLocal {
		preferred = memoryPropertyDeviceLocalBit
	}
	typ, ok := d.memoryType(req.memoryTypeBits, memoryPropertyHostVisibleBit|memoryPropertyHostCoherentBit, preferred)
	if !ok {
		b.Destroy()
		return nil, errors.New("no host-visible coherent memory type for storage buffers")
	}
	alloc := &memoryAllocateInfo{sType: stMemoryAllocateInfo, allocationSize: req.size, memoryTypeIndex: typ}
	if err := check(vkAllocateMemory(d.handle, alloc, nil, &b.memory), "vkAllocateMemory"); err != nil {
		b.Destroy()
		return nil, err
	}
	if err := check(vkBindBufferMemory(d.handle, b.handle, b.memory, 0), "vkBindBufferMemory"); err != nil {
		b.Destroy()
		return nil, err
	}
	var ptr unsafe.Pointer
	if err := check(vkMapMemory(d.handle, b.memory, 0, uint64(size), 0, &ptr), "vkMapMemory"); err != nil {
		b.Destroy()
		return nil, err
	}
	b.Data = unsafe.Slice((*byte)(ptr), size)
	return b, nil
}

// Destroy unmaps and frees the buffer. The device must be idle.
func (b *Buffer) Destroy() {
	h := b.dev.handle
	if b.Data != nil {
		vkUnmapMemory(h, b.memory)
		b.Data = nil
	}
	if b.handle != 0 {
		vkDestroyBuffer(h, b.handle, nil)
		b.handle = 0
	}
	if b.memory != 0 {
		vkFreeMemory(h, b.memory, nil)
		b.memory = 0
	}
}

// Kernel is a compute pipeline whose storage buffers are bound, in order, to
// bindings 0..n-1 of descriptor set 0.
type Kernel struct {
	dev        *Device
	module     uint64
	setLayout  uint64
	pipeLayout uint64
	pipeline   uint64
	pool       uint64
	set        uint64
	pushSize   uint32
	captured   bool // statistics captured (KernelOptions.CaptureStatistics)
}

// KernelOptions are optional pipeline settings. The zero value is the
// driver's default compilation.
type KernelOptions struct {
	// SubgroupSize, if nonzero, requires this subgroup size; the device must
	// be opened with DeviceOptions.SubgroupSizeControl. WorkgroupSize is the
	// shader's local_size_x, used to check the Vulkan limits.
	SubgroupSize  uint32
	WorkgroupSize uint32

	// CaptureStatistics keeps per-executable statistics for
	// Kernel.Executables. The device must be opened with
	// DeviceOptions.PipelineStatistics.
	CaptureStatistics bool
}

// NewKernel builds a compute pipeline from SPIR-V with entry point "main".
// specConstants[i] sets the 32-bit specialization constant with ID i.
func (d *Device) NewKernel(spirv []byte, buffers []*Buffer, pushSize uint32, specConstants []uint32, opts KernelOptions) (*Kernel, error) {
	if len(spirv) == 0 || len(spirv)%4 != 0 {
		return nil, errors.New("invalid SPIR-V module")
	}
	var pin runtime.Pinner
	defer pin.Unpin()

	k := &Kernel{dev: d, pushSize: pushSize}
	fail := func(err error) (*Kernel, error) {
		k.Destroy()
		return nil, err
	}

	code := make([]uint32, len(spirv)/4) // the driver needs 4-byte aligned code
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&code[0])), len(spirv)), spirv)
	pin.Pin(&code[0])
	modInfo := &shaderModuleCreateInfo{
		sType:    stShaderModuleCreateInfo,
		codeSize: uintptr(len(spirv)),
		pCode:    &code[0],
	}
	if err := check(vkCreateShaderModule(d.handle, modInfo, nil, &k.module), "vkCreateShaderModule"); err != nil {
		return fail(err)
	}

	bindings := make([]descriptorSetLayoutBinding, len(buffers))
	for i := range bindings {
		bindings[i] = descriptorSetLayoutBinding{
			binding:         uint32(i),
			descriptorType:  descriptorTypeStorageBuffer,
			descriptorCount: 1,
			stageFlags:      shaderStageComputeBit,
		}
	}
	pin.Pin(&bindings[0])
	dslInfo := &descriptorSetLayoutCreateInfo{
		sType:        stDescriptorSetLayoutCreate,
		bindingCount: uint32(len(bindings)),
		pBindings:    &bindings[0],
	}
	if err := check(vkCreateDescriptorSetLayout(d.handle, dslInfo, nil, &k.setLayout), "vkCreateDescriptorSetLayout"); err != nil {
		return fail(err)
	}

	setLayout := &k.setLayout
	pin.Pin(k)
	pushRange := &pushConstantRange{stageFlags: shaderStageComputeBit, size: pushSize}
	pin.Pin(pushRange)
	plInfo := &pipelineLayoutCreateInfo{
		sType:          stPipelineLayoutCreateInfo,
		setLayoutCount: 1,
		pSetLayouts:    setLayout,
	}
	if pushSize > 0 {
		plInfo.pushConstantRangeCount = 1
		plInfo.pPushConstantRanges = pushRange
	}
	if err := check(vkCreatePipelineLayout(d.handle, plInfo, nil, &k.pipeLayout), "vkCreatePipelineLayout"); err != nil {
		return fail(err)
	}

	entry := cstr("main")
	pin.Pin(entry)
	cpInfo := &computePipelineCreateInfo{
		sType: stComputePipelineCreateInfo,
		stage: pipelineShaderStageCreateInfo{
			sType:  stPipelineShaderStageCreate,
			stage:  shaderStageComputeBit,
			module: k.module,
			pName:  entry,
		},
		layout:            k.pipeLayout,
		basePipelineIndex: -1,
	}
	if len(specConstants) > 0 {
		entries := make([]specializationMapEntry, len(specConstants))
		for i := range entries {
			entries[i] = specializationMapEntry{constantID: uint32(i), offset: uint32(4 * i), size: 4}
		}
		data := slices.Clone(specConstants)
		spec := &specializationInfo{
			mapEntryCount: uint32(len(entries)),
			pMapEntries:   &entries[0],
			dataSize:      uintptr(4 * len(data)),
			pData:         unsafe.Pointer(&data[0]),
		}
		pin.Pin(&entries[0])
		pin.Pin(&data[0])
		pin.Pin(spec)
		cpInfo.stage.pSpecializationInfo = spec
	}
	if s := opts.SubgroupSize; s != 0 {
		info := d.Info
		switch {
		case !d.Options.SubgroupSizeControl:
			return fail(errors.New("subgroup size control is not enabled on this device"))
		case s&(s-1) != 0 || s < info.MinSubgroupSize || s > info.MaxSubgroupSize:
			return fail(fmt.Errorf("subgroup size %d not in %d..%d (powers of two)", s, info.MinSubgroupSize, info.MaxSubgroupSize))
		case uint64(opts.WorkgroupSize) > uint64(s)*uint64(info.maxWGSubgroups):
			return fail(fmt.Errorf("workgroup size %d exceeds %d subgroups of %d", opts.WorkgroupSize, info.maxWGSubgroups, s))
		}
		req := &pipelineShaderStageRequiredSubgroupSizeCreateInfo{sType: stPipelineShaderStageRequiredSubgroupSizeCreate, requiredSubgroupSize: s}
		pin.Pin(req)
		cpInfo.stage.pNext = unsafe.Pointer(req) // stage.flags stays 0 (ALLOW_VARYING must not be set)
	}
	if opts.CaptureStatistics {
		if !d.Options.PipelineStatistics {
			return fail(errors.New("pipeline statistics are not enabled on this device"))
		}
		cpInfo.flags |= pipelineCreateCaptureStatisticsBit
		k.captured = true
	}
	if err := check(vkCreateComputePipelines(d.handle, 0, 1, cpInfo, nil, &k.pipeline), "vkCreateComputePipelines"); err != nil {
		return fail(err)
	}

	poolSize := &descriptorPoolSize{typ: descriptorTypeStorageBuffer, descriptorCount: uint32(len(buffers))}
	pin.Pin(poolSize)
	poolInfo := &descriptorPoolCreateInfo{
		sType:         stDescriptorPoolCreateInfo,
		maxSets:       1,
		poolSizeCount: 1,
		pPoolSizes:    poolSize,
	}
	if err := check(vkCreateDescriptorPool(d.handle, poolInfo, nil, &k.pool), "vkCreateDescriptorPool"); err != nil {
		return fail(err)
	}
	setInfo := &descriptorSetAllocateInfo{
		sType:              stDescriptorSetAllocateInfo,
		descriptorPool:     k.pool,
		descriptorSetCount: 1,
		pSetLayouts:        setLayout,
	}
	if err := check(vkAllocateDescriptorSets(d.handle, setInfo, &k.set), "vkAllocateDescriptorSets"); err != nil {
		return fail(err)
	}

	infos := make([]descriptorBufferInfo, len(buffers))
	writes := make([]writeDescriptorSet, len(buffers))
	pin.Pin(&infos[0])
	for i, b := range buffers {
		infos[i] = descriptorBufferInfo{buffer: b.handle, rng: wholeSize}
		writes[i] = writeDescriptorSet{
			sType:           stWriteDescriptorSet,
			dstSet:          k.set,
			dstBinding:      uint32(i),
			descriptorCount: 1,
			descriptorType:  descriptorTypeStorageBuffer,
			pBufferInfo:     &infos[i],
		}
	}
	pin.Pin(&writes[0])
	vkUpdateDescriptorSets(d.handle, uint32(len(writes)), &writes[0], 0, nil)
	return k, nil
}

// Destroy destroys the pipeline and its resources. The device must be idle.
func (k *Kernel) Destroy() {
	h := k.dev.handle
	if k.pool != 0 {
		vkDestroyDescriptorPool(h, k.pool, nil)
		k.pool = 0
	}
	if k.pipeline != 0 {
		vkDestroyPipeline(h, k.pipeline, nil)
		k.pipeline = 0
	}
	if k.pipeLayout != 0 {
		vkDestroyPipelineLayout(h, k.pipeLayout, nil)
		k.pipeLayout = 0
	}
	if k.setLayout != 0 {
		vkDestroyDescriptorSetLayout(h, k.setLayout, nil)
		k.setLayout = 0
	}
	if k.module != 0 {
		vkDestroyShaderModule(h, k.module, nil)
		k.module = 0
	}
}

// Dispatch runs the kernel over groups workgroups and waits for it to finish,
// after which shader writes to the buffers are visible through Buffer.Data.
func (d *Device) Dispatch(k *Kernel, push []byte, groups uint32, timeout time.Duration) error {
	if uint32(len(push)) != k.pushSize {
		return fmt.Errorf("push constants are %d bytes, kernel expects %d", len(push), k.pushSize)
	}
	var pin runtime.Pinner
	defer pin.Unpin()

	if err := check(vkResetCommandBuffer(d.cmd, 0), "vkResetCommandBuffer"); err != nil {
		return err
	}
	begin := &commandBufferBeginInfo{sType: stCommandBufferBeginInfo, flags: commandBufferUsageOneTimeSubmitBit}
	if err := check(vkBeginCommandBuffer(d.cmd, begin), "vkBeginCommandBuffer"); err != nil {
		return err
	}
	vkCmdBindPipeline(d.cmd, pipelineBindPointCompute, k.pipeline)
	vkCmdBindDescriptorSets(d.cmd, pipelineBindPointCompute, k.pipeLayout, 0, 1, &k.set, 0, nil)
	if len(push) > 0 {
		vkCmdPushConstants(d.cmd, k.pipeLayout, shaderStageComputeBit, 0, k.pushSize, unsafe.Pointer(&push[0]))
	}
	vkCmdDispatch(d.cmd, groups, 1, 1)
	// Make shader writes available to host reads after the fence wait.
	barrier := &memoryBarrier{sType: stMemoryBarrier, srcAccessMask: accessShaderWriteBit, dstAccessMask: accessHostReadBit}
	vkCmdPipelineBarrier(d.cmd, pipelineStageComputeShaderBit, pipelineStageHostBit, 0, 1, barrier, 0, nil, 0, nil)
	if err := check(vkEndCommandBuffer(d.cmd), "vkEndCommandBuffer"); err != nil {
		return err
	}

	if err := check(vkResetFences(d.handle, 1, &d.fence), "vkResetFences"); err != nil {
		return err
	}
	cmd := &d.cmd
	pin.Pin(d)
	submit := &submitInfo{sType: stSubmitInfo, commandBufferCount: 1, pCommandBuffers: cmd}
	if err := check(vkQueueSubmit(d.queue, 1, submit, d.fence), "vkQueueSubmit"); err != nil {
		return err
	}
	r := vkWaitForFences(d.handle, 1, &d.fence, 1, uint64(timeout.Nanoseconds()))
	if r == resultTimeout {
		return fmt.Errorf("GPU did not finish within %v", timeout)
	}
	return check(r, "vkWaitForFences")
}
