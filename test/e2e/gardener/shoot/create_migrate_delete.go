// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package shoot

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	e2e "github.com/gardener/gardener/test/e2e/gardener"
	. "github.com/gardener/gardener/test/framework"
)

var _ = Describe("Shoot Tests", Label("Shoot", "control-plane-migration"), func() {
	test := func(shoot *gardencorev1beta1.Shoot) {
		f := defaultShootCreationFramework()
		f.Shoot = shoot

		// Assign seedName so that shoot does not get scheduled to the seed that will be used as target.
		f.Shoot.Spec.SeedName = ptr.To(getSeedName(false))

		It("Create, Migrate and Delete", Offset(1), func() {
			By("Create Shoot")
			ctx, cancel := context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancel()
			Expect(f.CreateShootAndWaitForCreation(ctx, false)).To(Succeed())
			f.Verify()

			By("Migrate Shoot")
			ctx, cancel = context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancel()
			t, err := newDefaultShootMigrationTest(ctx, f.Shoot, f.GardenerFramework)
			Expect(err).ToNot(HaveOccurred())
			Expect(t.MigrateShoot(ctx)).To(Succeed())
			Expect(t.VerifyMigration(ctx)).To(Succeed())

			By("Delete Shoot")
			ctx, cancel = context.WithTimeout(parentCtx, 15*time.Minute)
			defer cancel()
			Expect(f.DeleteShootAndWaitForDeletion(ctx, f.Shoot)).To(Succeed())
		})
	}

	Context("Shoot with workers", func() {
		test(e2e.DefaultShoot("e2e-migrate"))
	})

	Context("Workerless Shoot", Label("workerless"), func() {
		test(e2e.DefaultWorkerlessShoot("e2e-migrate"))
	})

	Context("Hibernated Shoot", Label("hibernated"), func() {
		shoot := e2e.DefaultShoot("e2e-mgr-hib")
		shoot.Spec.Hibernation = &gardencorev1beta1.Hibernation{
			Enabled: ptr.To(true),
		}
		test(shoot)
	})
})

func newDefaultShootMigrationTest(ctx context.Context, shoot *gardencorev1beta1.Shoot, gardenerFramework *GardenerFramework) (*ShootMigrationTest, error) {
	t, err := NewShootMigrationTest(ctx, gardenerFramework, &ShootMigrationConfig{
		ShootName:               shoot.Name,
		ShootNamespace:          shoot.Namespace,
		TargetSeedName:          getSeedName(true),
		SkipShootClientCreation: true,
		SkipNodeCheck:           true,
		SkipMachinesCheck:       true,
		SkipProtectedToleration: true,
	})
	return t, err
}

func getSeedName(isTarget bool) (seedName string) {
	switch os.Getenv("SHOOT_FAILURE_TOLERANCE_TYPE") {
	case "node":
		seedName = "local-ha-single-zone"
		if isTarget {
			seedName = "local2-ha-single-zone"
		}
	default:
		seedName = "local"
		if isTarget {
			seedName = "local2"
		}
	}

	return
}
