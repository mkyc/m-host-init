package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"

	hi "github.com/epiphany-platform/e-structures/hi/v0"
	st "github.com/epiphany-platform/e-structures/state/v0"
	"github.com/epiphany-platform/e-structures/utils/to"
	"github.com/spf13/viper"

	"github.com/spf13/cobra"
)

var (
	omitState bool

	vmsRsaPath        string
	useHostsPublicIPs bool
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	PreRun: func(cmd *cobra.Command, args []string) {
		logger.Debug().Msg("PreRun")

		err := viper.BindPFlags(cmd.Flags())
		if err != nil {
			logger.Fatal().Err(err).Msg("BindPFlags failed")
		}

		vmsRsaPath = viper.GetString("vms_rsa")
	},
	Run: func(cmd *cobra.Command, args []string) {
		logger.Debug().Msg("init called")
		moduleDirectoryPath := filepath.Join(SharedDirectory, moduleShortName)
		configFilePath := filepath.Join(SharedDirectory, moduleShortName, configFileName)
		stateFilePath := filepath.Join(SharedDirectory, stateFileName)
		logger.Debug().Msg("ensure directories")
		err := ensureDirectory(moduleDirectoryPath)
		if err != nil {
			logger.Fatal().Err(err).Msg("ensureDirectory failed")
		}
		logger.Debug().Msg("load state file")
		state, err := loadState(stateFilePath)
		if err != nil {
			logger.Fatal().Err(err).Msg("loadState failed")
		}
		// TODO probably get it out from here due to https://github.com/epiphany-platform/e-structures/issues/10
		if state.Hi == nil {
			state.Hi = &st.HiState{}
		}
		logger.Debug().Msg("load config file")
		config, err := loadConfig(configFilePath)
		if err != nil {
			logger.Fatal().Err(err).Msg("loadConfig failed")
		}

		if !reflect.DeepEqual(state.Hi, &st.HiState{}) && state.Hi.Status != st.Initialized && state.Hi.Status != st.Destroyed {
			logger.Fatal().Err(errors.New(string("unexpected state: " + state.Hi.Status))).Msg("incorrect state")
		}

		logger.Debug().Msg("backup state file")
		err = backupFile(stateFilePath)
		if err != nil {
			logger.Fatal().Err(err).Msg("backupFile failed")
		}
		logger.Debug().Msg("backup config file")
		err = backupFile(configFilePath)
		if err != nil {
			logger.Fatal().Err(err).Msg("backupFile failed")
		}

		config.GetParams().RsaPrivateKeyPath = to.StrPtr(filepath.Join(SharedDirectory, vmsRsaPath))

		if !omitState {
			if state.GetAzBIState().Status == st.Applied {

				hiVmGroups := make([]hi.VmGroup, 0, 0)
				for _, vmg := range state.GetAzBIState().GetOutput().GetVmGroups() {
					hiVmGroup := hi.VmGroup{
						Name:        vmg.Name,
						AdminUser:   to.StrPtr("operations"), //TODO extract it from AzBI config when https://github.com/epiphany-platform/m-azure-basic-infrastructure/issues/76 is done
						Hosts:       []hi.Host{},
						MountPoints: []hi.MountPoint{},
					}
					for _, outputDataDisk := range vmg.GetFirstVm().DataDisks {
						mountPoint := hi.MountPoint{
							Lun:  outputDataDisk.Lun,
							Path: to.StrPtr(fmt.Sprintf("/data/lun%d", *outputDataDisk.Lun)),
						}
						hiVmGroup.MountPoints = append(hiVmGroup.MountPoints, mountPoint)
					}

					for _, vm := range vmg.GetVms() {
						if useHostsPublicIPs {
							host := hi.Host{
								Name: vm.Name,
								Ip:   vm.PublicIp,
							}
							hiVmGroup.Hosts = append(hiVmGroup.Hosts, host)
						} else {
							if vm.PrivateIps != nil && len(vm.PrivateIps) > 0 {
								host := hi.Host{
									Name: vm.Name,
									Ip:   to.StrPtr(vm.PrivateIps[0]),
								}
								hiVmGroup.Hosts = append(hiVmGroup.Hosts, host)
							} else {
								logger.Warn().Msgf("host %s doesn't have private IPs", *vm.Name)
							}
						}
					}

					hiVmGroups = append(hiVmGroups, hiVmGroup)
				}
				config.GetParams().VmGroups = hiVmGroups

			}
		}

		state.Hi.Status = st.Initialized
		state.Hi.Config = config

		logger.Debug().Msg("save config")
		err = saveConfig(configFilePath, config)
		if err != nil {
			logger.Fatal().Err(err).Msg("saveConfig failed")
		}
		logger.Debug().Msg("save state")
		err = saveState(stateFilePath, state)
		if err != nil {
			logger.Fatal().Err(err).Msg("saveState failed")
		}

		bytes, err := config.Marshal()
		if err != nil {
			logger.Fatal().Err(err).Msg("config.Marshal failed")
		}
		logger.Info().Msg(string(bytes))
		fmt.Println("Initialized config: \n" + string(bytes))
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVarP(&omitState, "omit_state", "o", false, "omit state values during initialization")
	initCmd.Flags().BoolVarP(&useHostsPublicIPs, "use_public_ip", "p", false, "use public IP to access hosts")
	initCmd.Flags().String("vms_rsa", "vms_rsa", "name of rsa keypair to be provided to machines")
}
